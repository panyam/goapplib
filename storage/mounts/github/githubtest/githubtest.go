// Package githubtest is a fake GitHub for tests of code that reads and writes repos: the mounts
// GitHub driver, the storage example app, and consumers' own tests. It serves the REST routes the
// driver uses (a repo, a commit, a tarball, and the Contents API) over repos held in memory, and
// each write makes a real commit on its branch.
//
// A private repo answers only a request with an accepted token, as GitHub does, with 404 for
// anything else. Every request is recorded with the token it carried, so a test can check that a
// connection's token reached each one.
//
// oneauth's outbound/githubtest does a different job, minting and refreshing tokens. A test that
// drives a connection end to end uses both, with AcceptToken trusting the tokens the other mints.
package githubtest

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"sort"
	"strings"
	"sync"
	"testing"
)

// Server is the fake. Its URL is the API base a driver is pointed at.
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	repos    map[string]*Repo
	tokens   map[string]bool
	accept   func(token string) bool
	requests []Request
	fail     []int
	corrupt  bool
}

// Request is one request the fake answered.
type Request struct {
	Method string
	Path   string

	// Token is the bearer or "token" credential the request carried, or "" for none.
	Token string
}

// Repo is one repository in the fake. Its methods are safe to call while the server runs.
type Repo struct {
	s       *Server
	Owner   string
	Name    string
	Private bool

	defaultBranch string
	branches      map[string]string  // branch -> commit sha
	commits       map[string]*commit // sha -> commit
	extra         map[string][]TarEntry
	seq           int
}

type commit struct {
	sha   string
	files map[string]file
}

type file struct {
	data    []byte
	symlink bool
}

// TarEntry is an extra entry a test adds to a commit's tarball, such as a link or a path that
// climbs out, to check that a driver refuses it.
type TarEntry struct {
	Name     string // relative to the tarball's top folder
	Type     byte   // tar.TypeReg, tar.TypeSymlink, ...
	Body     string
	Linkname string
}

// New starts a fake and stops it when the test ends.
func New(t testing.TB) *Server {
	s := &Server{repos: map[string]*Repo{}, tokens: map[string]bool{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// AddToken makes tok an accepted token.
func (s *Server) AddToken(tok string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[tok] = true
}

// AcceptToken also accepts any token f approves, such as one oneauth's fake minted
// (its ValidInstallationToken or ValidUserToken).
func (s *Server) AcceptToken(f func(token string) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accept = f
}

// FailNext makes the next request answer with status and no body, then serve normally. 403 sets
// x-ratelimit-remaining to 0, as GitHub's rate limit does.
func (s *Server) FailNext(status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fail = append(s.fail, status)
}

// Requests returns every request answered so far, oldest first.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

// AddRepo creates owner/name with one commit on branch "main" holding files, and returns it.
func (s *Server) AddRepo(owner, name string, private bool, files map[string]string) *Repo {
	r := &Repo{s: s, Owner: owner, Name: name, Private: private, defaultBranch: "main",
		branches: map[string]string{}, commits: map[string]*commit{}, extra: map[string][]TarEntry{}}
	s.mu.Lock()
	s.repos[owner+"/"+name] = r
	s.mu.Unlock()
	r.Commit("main", files, nil)
	return r
}

// Source returns the repo's mounts source, "github://owner/name".
func (r *Repo) Source() string { return "github://" + r.Owner + "/" + r.Name }

// Head returns the commit sha branch points at, or "" for a missing branch.
func (r *Repo) Head(branch string) string {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	return r.branches[branch]
}

// Commit makes a commit on branch, creating the branch if needed. It starts from the branch's
// current files, sets each of put, and removes each of del. It returns the new commit's sha.
func (r *Repo) Commit(branch string, put map[string]string, del []string) string {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	files := map[string]file{}
	if head, ok := r.branches[branch]; ok {
		for p, f := range r.commits[head].files {
			files[p] = f
		}
	}
	for p, d := range put {
		files[p] = file{data: []byte(d)}
	}
	for _, p := range del {
		delete(files, p)
	}
	return r.commitLocked(branch, files)
}

// Symlink makes a commit on branch adding a link at p to target.
func (r *Repo) Symlink(branch, p, target string) string {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	files := map[string]file{}
	for q, f := range r.commits[r.branches[branch]].files {
		files[q] = f
	}
	files[p] = file{data: []byte(target), symlink: true}
	return r.commitLocked(branch, files)
}

// AddTarEntries adds entries to the tarball of commit sha only. The Contents API doesn't see them.
func (r *Repo) AddTarEntries(sha string, es ...TarEntry) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	r.extra[sha] = append(r.extra[sha], es...)
}

// File returns the contents of p at the head of branch.
func (r *Repo) File(branch, p string) (string, bool) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	c := r.commits[r.branches[branch]]
	if c == nil {
		return "", false
	}
	f, ok := c.files[p]
	return string(f.data), ok
}

func (r *Repo) commitLocked(branch string, files map[string]file) string {
	r.seq++
	h := sha1.New()
	fmt.Fprintf(h, "%s/%s %d %s\n", r.Owner, r.Name, r.seq, r.branches[branch])
	for _, p := range sortedKeys(files) {
		fmt.Fprintf(h, "%s %s\n", p, blobSHA(files[p].data))
	}
	sha := hex.EncodeToString(h.Sum(nil))
	r.commits[sha] = &commit{sha: sha, files: files}
	r.branches[branch] = sha
	return sha
}

// blobSHA is git's blob id: SHA-1 of "blob <len>\0" and the contents.
func blobSHA(b []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(b))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

func sortedKeys(m map[string]file) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func token(req *http.Request) string {
	a := req.Header.Get("Authorization")
	for _, p := range []string{"Bearer ", "bearer ", "token "} {
		if t, ok := strings.CutPrefix(a, p); ok {
			return t
		}
	}
	return ""
}

func (s *Server) serve(w http.ResponseWriter, req *http.Request) {
	tok := token(req)
	s.mu.Lock()
	s.requests = append(s.requests, Request{Method: req.Method, Path: req.URL.Path, Token: tok})
	var fail int
	if len(s.fail) > 0 {
		fail, s.fail = s.fail[0], s.fail[1:]
	}
	accepted := tok != "" && (s.tokens[tok] || s.accept != nil && s.accept(tok))
	s.mu.Unlock()

	if fail != 0 {
		if fail == http.StatusForbidden {
			w.Header().Set("x-ratelimit-remaining", "0")
		}
		w.WriteHeader(fail)
		return
	}
	if tok != "" && !accepted {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "Bad credentials"})
		return
	}

	if raw, ok := strings.CutPrefix(req.URL.Path, "/raw/"); ok && req.Method == http.MethodGet {
		s.raw(w, raw, accepted)
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(req.URL.Path, "/repos/"), "/", 4)
	if !strings.HasPrefix(req.URL.Path, "/repos/") || len(parts) < 2 {
		notFound(w)
		return
	}
	s.mu.Lock()
	r := s.repos[parts[0]+"/"+parts[1]]
	s.mu.Unlock()
	if r == nil || r.Private && !accepted {
		notFound(w)
		return
	}
	switch {
	case len(parts) == 2 && req.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"full_name": r.Owner + "/" + r.Name, "private": r.Private, "default_branch": r.defaultBranch})
	case len(parts) == 4 && parts[2] == "commits" && req.Method == http.MethodGet:
		r.getCommit(w, parts[3])
	case len(parts) == 4 && parts[2] == "tarball" && req.Method == http.MethodGet:
		r.tarball(w, parts[3])
	case len(parts) == 4 && parts[2] == "branches" && req.Method == http.MethodGet:
		r.getBranch(w, parts[3])
	case len(parts) == 4 && parts[2] == "git" && strings.HasPrefix(parts[3], "trees/") && req.Method == http.MethodGet:
		r.getTree(w, strings.TrimPrefix(parts[3], "trees/"), req.URL.Query().Get("recursive") == "1")
	case len(parts) == 4 && parts[2] == "git" && strings.HasPrefix(parts[3], "blobs/") && req.Method == http.MethodGet:
		r.getBlob(w, strings.TrimPrefix(parts[3], "blobs/"))
	case len(parts) >= 3 && parts[2] == "contents":
		p := ""
		if len(parts) == 4 {
			p = strings.Trim(parts[3], "/")
		}
		switch req.Method {
		case http.MethodGet:
			r.getContents(w, p, req.URL.Query().Get("ref"))
		case http.MethodPut:
			r.putContents(w, req, p)
		case http.MethodDelete:
			r.deleteContents(w, req, p)
		default:
			notFound(w)
		}
	default:
		notFound(w)
	}
}

// RawURL is the fake's raw host, which serves a file by owner, repo, commit and path as
// raw.githubusercontent.com does. Point a driver at it with WithRawURL.
func (s *Server) RawURL() string { return s.URL + "/raw" }

// CorruptRaw makes the raw host and the git blobs endpoint send each file with one byte added, so
// a test can check that a driver refuses bytes that don't match their git blob id.
func (s *Server) CorruptRaw(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.corrupt = on
}

// raw serves "<owner>/<repo>/<commit>/<path>". A private repo needs an accepted token, and answers
// 404 without one, as GitHub does.
func (s *Server) raw(w http.ResponseWriter, p string, accepted bool) {
	parts := strings.SplitN(p, "/", 4)
	if len(parts) < 4 {
		notFound(w)
		return
	}
	s.mu.Lock()
	r := s.repos[parts[0]+"/"+parts[1]]
	s.mu.Unlock()
	if r == nil || r.Private && !accepted {
		notFound(w)
		return
	}
	c := r.resolve(parts[2])
	if c == nil {
		notFound(w)
		return
	}
	f, ok := c.files[parts[3]]
	if !ok || f.symlink {
		notFound(w)
		return
	}
	s.writeFile(w, f.data)
}

func (s *Server) writeFile(w http.ResponseWriter, data []byte) {
	s.mu.Lock()
	corrupt := s.corrupt
	s.mu.Unlock()
	if corrupt {
		data = append(append([]byte(nil), data...), '!')
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

func (r *Repo) getBranch(w http.ResponseWriter, name string) {
	r.s.mu.Lock()
	sha, ok := r.branches[name]
	r.s.mu.Unlock()
	if !ok {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "commit": map[string]string{"sha": sha}})
}

// getTree answers the trees API for a commit, every file with its git blob id, plus a folder
// entry for each folder, as GitHub lists them. A link has mode 120000.
func (r *Repo) getTree(w http.ResponseWriter, ref string, recursive bool) {
	c := r.resolve(ref)
	if c == nil || !recursive {
		notFound(w)
		return
	}
	type treeEntry struct {
		Path string `json:"path"`
		Mode string `json:"mode"`
		Type string `json:"type"`
		SHA  string `json:"sha"`
		Size int    `json:"size,omitempty"`
	}
	var es []treeEntry
	dirs := map[string]bool{}
	for _, p := range sortedKeys(c.files) {
		for d := path.Dir(p); d != "." && !dirs[d]; d = path.Dir(d) {
			dirs[d] = true
			es = append(es, treeEntry{Path: d, Mode: "040000", Type: "tree", SHA: blobSHA([]byte(d))})
		}
		f := c.files[p]
		mode := "100644"
		if f.symlink {
			mode = "120000"
		}
		es = append(es, treeEntry{Path: p, Mode: mode, Type: "blob", SHA: blobSHA(f.data), Size: len(f.data)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"sha": c.sha, "tree": es, "truncated": false})
}

// getBlob serves a blob's raw bytes by its git blob id, as the git blobs API does with the raw
// media type.
func (r *Repo) getBlob(w http.ResponseWriter, id string) {
	r.s.mu.Lock()
	var data []byte
	found := false
	for _, c := range r.commits {
		for _, f := range c.files {
			if !f.symlink && blobSHA(f.data) == id {
				data, found = f.data, true
				break
			}
		}
		if found {
			break
		}
	}
	r.s.mu.Unlock()
	if !found {
		notFound(w)
		return
	}
	r.s.writeFile(w, data)
}

// resolve turns a branch name or a full commit sha into a commit.
func (r *Repo) resolve(ref string) *commit {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if ref == "" {
		ref = r.defaultBranch
	}
	if sha, ok := r.branches[ref]; ok {
		return r.commits[sha]
	}
	return r.commits[ref]
}

func (r *Repo) getCommit(w http.ResponseWriter, ref string) {
	c := r.resolve(ref)
	if c == nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": "No commit found for SHA: " + ref})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sha": c.sha})
}

func (r *Repo) tarball(w http.ResponseWriter, ref string) {
	c := r.resolve(ref)
	if c == nil {
		notFound(w)
		return
	}
	r.s.mu.Lock()
	extra := append([]TarEntry(nil), r.extra[c.sha]...)
	r.s.mu.Unlock()
	top := fmt.Sprintf("%s-%s-%s/", r.Owner, r.Name, c.sha[:7])
	w.Header().Set("Content-Type", "application/x-gzip")
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: top, Typeflag: tar.TypeDir, Mode: 0o755})
	dirs := map[string]bool{}
	for _, p := range sortedKeys(c.files) {
		for d := path.Dir(p); d != "." && !dirs[d]; d = path.Dir(d) {
			dirs[d] = true
			tw.WriteHeader(&tar.Header{Name: top + d + "/", Typeflag: tar.TypeDir, Mode: 0o755})
		}
		f := c.files[p]
		if f.symlink {
			tw.WriteHeader(&tar.Header{Name: top + p, Typeflag: tar.TypeSymlink, Linkname: string(f.data), Mode: 0o777})
			continue
		}
		tw.WriteHeader(&tar.Header{Name: top + p, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(f.data))})
		tw.Write(f.data)
	}
	for _, e := range extra {
		tw.WriteHeader(&tar.Header{Name: top + e.Name, Typeflag: e.Type, Linkname: e.Linkname, Mode: 0o644, Size: int64(len(e.Body))})
		if e.Type == tar.TypeReg {
			tw.Write([]byte(e.Body))
		}
	}
	tw.Close()
	gz.Close()
}

type entry struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	SHA      string `json:"sha"`
	Size     int    `json:"size"`
	Content  string `json:"content,omitempty"`
	Encoding string `json:"encoding,omitempty"`
}

func (r *Repo) getContents(w http.ResponseWriter, p, ref string) {
	c := r.resolve(ref)
	if c == nil {
		notFound(w)
		return
	}
	if f, ok := c.files[p]; ok && p != "" {
		e := entry{Type: "file", Name: path.Base(p), Path: p, SHA: blobSHA(f.data), Size: len(f.data),
			Content: base64.StdEncoding.EncodeToString(f.data), Encoding: "base64"}
		if f.symlink {
			e.Type, e.Content, e.Encoding = "symlink", "", ""
		}
		writeJSON(w, http.StatusOK, e)
		return
	}
	prefix := ""
	if p != "" {
		prefix = p + "/"
	}
	seen := map[string]bool{}
	var list []entry
	for _, q := range sortedKeys(c.files) {
		rest, ok := strings.CutPrefix(q, prefix)
		if !ok {
			continue
		}
		name, sub, isDir := strings.Cut(rest, "/")
		_ = sub
		if seen[name] {
			continue
		}
		seen[name] = true
		f := c.files[q]
		e := entry{Type: "file", Name: name, Path: prefix + name, SHA: blobSHA(f.data), Size: len(f.data)}
		switch {
		case isDir:
			e = entry{Type: "dir", Name: name, Path: prefix + name}
		case f.symlink:
			e.Type = "symlink"
		}
		list = append(list, e)
	}
	if p != "" && len(list) == 0 {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type writeBody struct {
	Message string `json:"message"`
	Content string `json:"content"`
	SHA     string `json:"sha"`
	Branch  string `json:"branch"`
}

func (r *Repo) readWrite(w http.ResponseWriter, req *http.Request, p string) (writeBody, *commit, bool) {
	var b writeBody
	if err := json.NewDecoder(req.Body).Decode(&b); err != nil || p == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": "Invalid request."})
		return b, nil, false
	}
	if b.Branch == "" {
		b.Branch = r.defaultBranch
	}
	r.s.mu.Lock()
	head := r.commits[r.branches[b.Branch]]
	r.s.mu.Unlock()
	if head == nil {
		notFound(w)
		return b, nil, false
	}
	return b, head, true
}

func (r *Repo) putContents(w http.ResponseWriter, req *http.Request, p string) {
	b, head, ok := r.readWrite(w, req, p)
	if !ok {
		return
	}
	data, err := base64.StdEncoding.DecodeString(b.Content)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": "content is not valid Base64"})
		return
	}
	cur, exists := head.files[p]
	switch {
	case exists && b.SHA == "":
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": `Invalid request. "sha" wasn't supplied.`})
		return
	case exists && b.SHA != blobSHA(cur.data), !exists && b.SHA != "":
		writeJSON(w, http.StatusConflict, map[string]string{"message": p + " does not match " + b.SHA})
		return
	}
	sha := r.Commit(b.Branch, map[string]string{p: string(data)}, nil)
	status := http.StatusOK
	if !exists {
		status = http.StatusCreated
	}
	writeJSON(w, status, map[string]any{
		"content": entry{Type: "file", Name: path.Base(p), Path: p, SHA: blobSHA(data), Size: len(data)},
		"commit":  map[string]string{"sha": sha, "message": b.Message},
	})
}

func (r *Repo) deleteContents(w http.ResponseWriter, req *http.Request, p string) {
	b, head, ok := r.readWrite(w, req, p)
	if !ok {
		return
	}
	cur, exists := head.files[p]
	switch {
	case !exists:
		notFound(w)
		return
	case b.SHA != blobSHA(cur.data):
		writeJSON(w, http.StatusConflict, map[string]string{"message": p + " does not match " + b.SHA})
		return
	}
	sha := r.Commit(b.Branch, nil, []string{p})
	writeJSON(w, http.StatusOK, map[string]any{"content": nil, "commit": map[string]string{"sha": sha, "message": b.Message}})
}

func notFound(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, map[string]string{"message": "Not Found"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
