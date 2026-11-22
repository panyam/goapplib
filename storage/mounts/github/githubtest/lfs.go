package githubtest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// lfsState is the fake's Git LFS storage. Objects are shared by every repo, keyed by oid, as a
// test needs no more.
type lfsState struct {
	objects map[string][]byte
	fail    []lfsFailure
	corrupt bool
}

type lfsFailure struct {
	status  int
	message string
}

// LFSURL is the web host whose "/{owner}/{repo}.git/info/lfs/objects/batch" serves the LFS batch
// API. Point a driver at it with WithLFSURL.
func (s *Server) LFSURL() string { return s.URL }

// LFSPointer returns the Git LFS pointer file for content, and its oid.
func LFSPointer(content string) (pointer, oid string) {
	sum := sha256.Sum256([]byte(content))
	oid = hex.EncodeToString(sum[:])
	return fmt.Sprintf("version https://git-lfs.github.com/spec/v1\noid sha256:%s\nsize %d\n", oid, len(content)), oid
}

// CommitLFS commits p on branch as a Git LFS pointer to content, and keeps content as its LFS
// object. It returns the commit and the object's oid.
func (r *Repo) CommitLFS(branch, p, content string) (sha, oid string) {
	ptr, oid := LFSPointer(content)
	r.s.mu.Lock()
	r.s.lfs.objects[oid] = []byte(content)
	r.s.mu.Unlock()
	return r.Commit(branch, map[string]string{p: ptr}, nil), oid
}

// RemoveLFSObject deletes an object, so the batch API reports it missing.
func (s *Server) RemoveLFSObject(oid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.lfs.objects, oid)
}

// FailLFS makes the next batch request answer with status and message, such as 403 with
// GitHub's quota message.
func (s *Server) FailLFS(status int, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lfs.fail = append(s.lfs.fail, lfsFailure{status, message})
}

// CorruptLFS makes each object download with one byte added, so a test can check that a driver
// refuses bytes that don't match the pointer.
func (s *Server) CorruptLFS(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lfs.corrupt = on
}

// lfsBatch serves "/{owner}/{repo}.git/info/lfs/objects/batch" for downloads. Each href goes to
// /lfs-objects/ and needs the header the answer gives it, as GitHub's signed hrefs do.
func (s *Server) lfsBatch(w http.ResponseWriter, req *http.Request, p, tok string, accepted bool) {
	name := strings.TrimSuffix(strings.TrimPrefix(p, "/"), ".git/info/lfs/objects/batch")
	s.mu.Lock()
	r := s.repos[strings.ToLower(name)]
	var fail *lfsFailure
	if len(s.lfs.fail) > 0 {
		fail = &s.lfs.fail[0]
		s.lfs.fail = s.lfs.fail[1:]
	}
	s.mu.Unlock()
	if fail != nil {
		writeJSON(w, fail.status, map[string]string{"message": fail.message})
		return
	}
	if !s.visible(r, tok, accepted) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "Repository not found"})
		return
	}
	var body struct {
		Operation string `json:"operation"`
		Objects   []struct {
			OID  string `json:"oid"`
			Size int64  `json:"size"`
		} `json:"objects"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Operation != "download" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": "bad batch request"})
		return
	}
	var out []map[string]any
	for _, o := range body.Objects {
		s.mu.Lock()
		_, ok := s.lfs.objects[o.OID]
		s.mu.Unlock()
		obj := map[string]any{"oid": o.OID, "size": o.Size}
		if ok {
			obj["actions"] = map[string]any{"download": map[string]any{
				"href":   s.URL + "/lfs-objects/" + o.OID,
				"header": map[string]string{"Authorization": "RemoteAuth " + o.OID},
			}}
		} else {
			obj["error"] = map[string]any{"code": http.StatusNotFound, "message": "Object does not exist"}
		}
		out = append(out, obj)
	}
	w.Header().Set("Content-Type", "application/vnd.git-lfs+json")
	writeJSON(w, http.StatusOK, map[string]any{"transfer": "basic", "objects": out})
}

func (s *Server) lfsObject(w http.ResponseWriter, req *http.Request, oid string) {
	s.mu.Lock()
	b, ok := s.lfs.objects[oid]
	corrupt := s.lfs.corrupt
	s.mu.Unlock()
	if !ok || req.Header.Get("Authorization") != "RemoteAuth "+oid {
		notFound(w)
		return
	}
	if corrupt {
		b = append(append([]byte(nil), b...), 'x')
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write(b)
}
