package service

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/panyam/goapplib/wasmhost"
)

func do(t *testing.T, h http.Handler, method, path, body string, ctx context.Context) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func state(t *testing.T, h http.Handler) State {
	t.Helper()
	var st State
	if err := json.Unmarshal(do(t, h, "GET", "/state", "", context.Background()).Body.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	return st
}

func TestIngestIsDeterministicAndCounted(t *testing.T) {
	a, b := (&Service{}).Handler(), (&Service{}).Handler()
	for _, h := range []http.Handler{a, b} {
		if rec := do(t, h, "POST", "/ingest", `{"peakMB": 4, "resultMB": 1}`, context.Background()); rec.Code != http.StatusOK {
			t.Fatalf("ingest: %d %s", rec.Code, rec.Body)
		}
	}
	sa, sb := state(t, a), state(t, b)
	if sa.Checksum == "" || sa.Checksum != sb.Checksum || sa.ResultBytes != 1<<20 || sa.IngestCount != 1 {
		t.Fatalf("states %+v and %+v; want the same checksum, 1 MB, one ingest", sa, sb)
	}
	do(t, a, "POST", "/ingest", `{"peakMB": 4, "resultMB": 1}`, context.Background())
	if got := state(t, a).IngestCount; got != 2 {
		t.Fatalf("ingest count %d after two ingests", got)
	}
	if rec := do(t, a, "POST", "/ingest", `{"peakMB": 1, "resultMB": 2}`, context.Background()); rec.Code != http.StatusBadRequest {
		t.Fatalf("a result bigger than the peak: %d", rec.Code)
	}
}

func TestQueryAnswersFromTheState(t *testing.T) {
	h := (&Service{}).Handler()
	if !strings.Contains(do(t, h, "POST", "/query", "", context.Background()).Body.String(), `"ok":false`) {
		t.Fatal("query before ingest should say ok false")
	}
	do(t, h, "POST", "/ingest", `{"peakMB": 2, "resultMB": 1}`, context.Background())
	if !strings.Contains(do(t, h, "POST", "/query", "", context.Background()).Body.String(), `"ok":true`) {
		t.Fatal("query after ingest should say ok true")
	}
}

func TestLongJobReportsProgressAndHowItEnded(t *testing.T) {
	h := (&Service{}).Handler()
	rec := do(t, h, "POST", "/longjob", `{"ms": 700}`, context.Background())
	var progress, done int
	sc := bufio.NewScanner(rec.Body)
	for sc.Scan() {
		switch {
		case strings.Contains(sc.Text(), `"progress"`):
			progress++
		case strings.Contains(sc.Text(), `"done":"completed"`):
			done++
		}
	}
	if progress < 2 || done != 1 {
		t.Fatalf("%d progress lines and %d completed lines in:\n%s", progress, done, rec.Body)
	}
	if got := state(t, h).LastJob; got != "completed" {
		t.Fatalf("lastJob %q", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if body := do(t, h, "POST", "/longjob", `{"ms": 5000}`, ctx).Body.String(); !strings.Contains(body, `"done":"cancelled"`) {
		t.Fatalf("a cancelled job wrote:\n%s", body)
	}
	if got := state(t, h).LastJob; got != "cancelled" {
		t.Fatalf("lastJob %q after a cancelled job", got)
	}
}

func TestOpenRestoresFromTheCacheInsteadOfIngesting(t *testing.T) {
	cache := &wasmhost.MemCache{}
	first := (&Service{Cache: cache}).Handler()
	do(t, first, "POST", "/open", `{"peakMB": 4, "resultMB": 1}`, context.Background())
	a := state(t, first)
	if a.IngestCount != 1 || a.Restored || a.CacheError != "" {
		t.Fatalf("first open: %+v; want one ingest, not restored", a)
	}

	second := (&Service{Cache: cache}).Handler()
	do(t, second, "POST", "/open", `{"peakMB": 4, "resultMB": 1}`, context.Background())
	b := state(t, second)
	if b.IngestCount != 0 || !b.Restored || b.Checksum != a.Checksum || b.ResultBytes != a.ResultBytes {
		t.Fatalf("second open over the same cache: %+v; want restored with checksum %s and no ingest", b, a.Checksum)
	}
	if !strings.Contains(do(t, second, "POST", "/query", "", context.Background()).Body.String(), `"ok":true`) {
		t.Fatal("a restored service should answer queries")
	}

	do(t, second, "POST", "/open", `{"peakMB": 4, "resultMB": 2}`, context.Background())
	if c := state(t, second); c.IngestCount != 1 || c.Restored {
		t.Fatalf("different inputs: %+v; want an ingest", c)
	}
}
