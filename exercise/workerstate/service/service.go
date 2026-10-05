// Package service is the toy stateful service behind mission #74's exercise (goapplib issue 75).
// Its ingest builds a compact result through a much larger scratch allocation, the way agni's
// engine parses and lays out a design, and it keeps the result between requests. The wasm main
// serves it from a Web Worker through wasmhost; the tests serve it natively.
//
// Plain JSON over net/http rather than Connect: the mission is about state, memory and
// concurrency, and exercise/wasmhost already covers Connect over workerFetch.
package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/panyam/goapplib/wasmhost"
)

// State is what GET /state reports.
type State struct {
	// IngestCount is how many ingests this process has run. A state restored from the cache
	// rather than rebuilt leaves it at 0.
	IngestCount int    `json:"ingestCount"`
	ResultBytes int    `json:"resultBytes"`
	Checksum    string `json:"checksum"`
	// LastJob is how the last /longjob ended: "", "completed" or "cancelled".
	LastJob string `json:"lastJob"`
	// Restored says the result came from the cache rather than an ingest.
	Restored bool `json:"restored"`
	// CacheError is the last cache failure other than a miss, for the exercise to show.
	CacheError string `json:"cacheError,omitempty"`
}

// Service holds the result of the last ingest. Cache, when set, is where /open looks for a result
// before ingesting, and where it keeps one after.
type Service struct {
	Cache wasmhost.Cache

	mu     sync.Mutex
	result []byte
	state  State
}

// IngestRequest sizes an ingest: PeakMB of scratch on the way to a ResultMB result.
type IngestRequest struct {
	PeakMB   int `json:"peakMB"`
	ResultMB int `json:"resultMB"`
}

// Handler serves the service's four routes.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /ingest", s.ingest)
	mux.HandleFunc("POST /open", s.open)
	mux.HandleFunc("GET /state", s.getState)
	mux.HandleFunc("POST /query", s.query)
	mux.HandleFunc("POST /longjob", s.longJob)
	return mux
}

// ingest touches every byte of a PeakMB scratch buffer and derives a ResultMB result from it,
// deterministically, so the same request always gives the same checksum. The scratch is garbage
// afterwards, but wasm linear memory never shrinks, which is what the exercise measures.
func (s *Service) ingest(w http.ResponseWriter, r *http.Request) {
	req, ok := readIngest(w, r)
	if !ok {
		return
	}
	writeJSON(w, s.build(req))
}

// open is what a page calls on load: the result for these inputs from the cache if it's there,
// and otherwise an ingest, whose result then goes into the cache for the next load.
func (s *Service) open(w http.ResponseWriter, r *http.Request) {
	req, ok := readIngest(w, r)
	if !ok {
		return
	}
	if s.Cache == nil {
		writeJSON(w, s.build(req))
		return
	}
	key := wasmhost.CacheKey([]byte("workerstate-v1"), []byte(strconv.Itoa(req.PeakMB)), []byte(strconv.Itoa(req.ResultMB)))
	b, err := s.Cache.Get(r.Context(), key)
	if err == nil {
		writeJSON(w, s.keep(b, true))
		return
	}
	st := s.build(req)
	if !errors.Is(err, wasmhost.ErrMiss) {
		st = s.cacheFailed(err)
	} else if err := s.Cache.Put(r.Context(), key, s.current()); err != nil {
		st = s.cacheFailed(err)
	}
	writeJSON(w, st)
}

func readIngest(w http.ResponseWriter, r *http.Request) (IngestRequest, bool) {
	var req IngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.PeakMB <= 0 || req.ResultMB <= 0 || req.ResultMB > req.PeakMB {
		http.Error(w, "want {peakMB, resultMB} with 0 < resultMB <= peakMB", http.StatusBadRequest)
		return req, false
	}
	return req, true
}

func (s *Service) build(req IngestRequest) State {
	scratch := make([]byte, req.PeakMB<<20)
	for i := range scratch {
		scratch[i] = byte(i*31 + i>>8)
	}
	result := make([]byte, req.ResultMB<<20)
	step := len(scratch) / len(result)
	for i := range result {
		result[i] = scratch[i*step] ^ byte(i>>4)
	}
	return s.keep(result, false)
}

// keep makes result the service's state. An ingest counts; a restore doesn't.
func (s *Service) keep(result []byte, restored bool) State {
	sum := sha256.Sum256(result)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.result = result
	if !restored {
		s.state.IngestCount++
	}
	s.state.Restored = restored
	s.state.ResultBytes = len(result)
	s.state.Checksum = hex.EncodeToString(sum[:])
	return s.state
}

func (s *Service) current() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.result
}

func (s *Service) cacheFailed(err error) State {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.CacheError = err.Error()
	return s.state
}

func (s *Service) getState(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	st := s.state
	s.mu.Unlock()
	writeJSON(w, st)
}

// query is the cheap request: one byte of the result.
func (s *Service) query(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.result) == 0 {
		writeJSON(w, map[string]any{"ok": false})
		return
	}
	writeJSON(w, map[string]any{"ok": true, "byte": s.result[len(s.result)/2]})
}

// longJob runs a CPU loop for ms milliseconds without yielding, like a layout job, checking its
// request's context between slices. Every 250 ms it writes a progress line (NDJSON) and flushes, if
// the response can. It ends with a {"done": ...} line and records how it ended in State.LastJob.
func (s *Service) longJob(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MS int `json:"ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MS <= 0 {
		http.Error(w, "want {ms}", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	start := time.Now()
	nextProgress := 250 * time.Millisecond
	end := "completed"
	var n uint64
	for {
		for i := 0; i < 200_000; i++ {
			n += uint64(i) ^ n>>3
		}
		el := time.Since(start)
		if r.Context().Err() != nil {
			end = "cancelled"
			break
		}
		if el >= time.Duration(req.MS)*time.Millisecond {
			break
		}
		if el >= nextProgress {
			_ = enc.Encode(map[string]any{"progress": float64(el) / float64(time.Duration(req.MS)*time.Millisecond)})
			if flusher != nil {
				flusher.Flush()
			}
			nextProgress += 250 * time.Millisecond
		}
	}
	s.mu.Lock()
	s.state.LastJob = end
	s.mu.Unlock()
	_ = enc.Encode(map[string]any{"done": end, "ms": time.Since(start).Milliseconds(), "n": n})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
