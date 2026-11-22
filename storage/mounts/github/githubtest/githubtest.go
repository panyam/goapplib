// Package githubtest is a fake GitHub for tests of code that reads and writes repos: the mounts
// GitHub driver, the storage example app, and consumers' own tests. It serves the REST routes the
// driver uses (a repo, a commit, a branch's commits, the tags, a tarball, and the Contents API) and the
// GraphQL tags query over repos held in memory, and each write makes a real commit on its branch.
// It serves the hooks API too, and Push delivers a signed push to each hook made through it.
// PushRequest and SignedRequest build signed webhook deliveries for an app's handler.
//
// A private repo answers only a request with an accepted token, as GitHub does, with 404 for
// anything else. Once a private repo has grants (Repo.Grant), only a granted token reads it. Every request is recorded with the token it carried, so a test can check that a
// connection's token reached each one.
//
// It serves the Git LFS batch API for downloads (lfs.go), so a repo can hold LFS files, whose
// tarball, raw and contents answers carry only the pointer, as GitHub's do.
//
// It serves a GitHub App's sign-in on the same host, at the paths oneauth's
// GithubOAuth2.SetEnterpriseURL points at: the web flow with PKCE, the device flow, refresh, /user
// and org membership. AddApp registers the App, AddUser makes a person, and SignInAs or
// User.Approve stands in for the person in the browser. Each sign-in issues the person a token,
// which Repo.GrantUser and Repo.HookManagerUsers name by login. Every API route also answers under
// /api/v3, as on a GitHub Enterprise Server host.
//
// oneauth's outbound/githubtest mints installation tokens and user tokens for a connection, with
// no sign-in or repos. A test that drives a connection end to end uses both, with AcceptToken
// trusting the tokens the other mints.
package githubtest

import (
	"archive/tar"
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Server is the fake. Its URL is the API base a driver is pointed at, and the origin of every URL
// the fake hands out.
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	repos    map[string]*Repo // by lowercase owner/name, since GitHub ignores case there
	tokens   map[string]bool
	accept   func(token string) bool
	requests []Request
	fail     []int
	corrupt  bool
	hookSeq  int64
	lfs      lfsState

	// Sign-in state, in signin.go.
	apps           map[string]string // client id -> secret
	users          map[string]*User  // by lowercase login
	userSeq        int64
	signedIn       *User
	codes          map[string]authCode
	access         map[string]*User        // live sign-in access token -> person
	issuedBy       map[string]string       // sign-in access token -> its refresh token
	refresh        map[string]refreshGrant // unused refresh token -> grant
	devices        map[string]*device      // device code -> sign-in
	deviceInterval time.Duration

	listenURL string
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
	tags          map[string]tag
	seq           int
	grants        map[string]bool // guarded by s.mu
	userGrants    map[string]bool // lowercase logins, guarded by s.mu
	hooks         []*Hook         // guarded by s.mu
	managers      map[string]bool // guarded by s.mu
	userManagers  map[string]bool // lowercase logins, guarded by s.mu
}

type commit struct {
	sha     string
	parent  string
	message string
	date    time.Time
	files   map[string]file
}

// tag is a tag in the fake. An annotated tag points at a tag object, which GraphQL shows as a Tag
// whose target is the commit.
type tag struct {
	sha       string
	annotated bool
}

// epoch is the date of every fake repo's first commit. Each later commit is a minute after the one
// before, unless a test sets its date with SetCommitTime.
var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

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

// Option configures a fake that New starts.
type Option func(*options)

type options struct {
	listener net.Listener
	baseURL  string
}

// WithListener serves the fake on l instead of a fresh 127.0.0.1 port. A test whose app runs in a
// container listens on "0.0.0.0:0" or the docker bridge's gateway, so the container can reach it.
// The fake closes l when the test ends.
func WithListener(l net.Listener) Option { return func(o *options) { o.listener = l } }

// WithBaseURL sets the origin the fake puts in every URL it hands out: Server.URL, RawURL, LFSURL,
// the LFS batch hrefs, pagination Link headers and the device flow's verification_uri. Use an
// address the app under test can reach, such as "http://host.docker.internal:<port>". u is a scheme,
// a host and an optional port. New fails the test if u has a path, query or fragment. The test
// process itself may not resolve u, so it reaches the fake through ListenURL.
func WithBaseURL(u string) Option { return func(o *options) { o.baseURL = u } }

// New starts a fake and stops it when the test ends. With no options it listens on a fresh
// 127.0.0.1 port, and URL is that address.
func New(t testing.TB, opts ...Option) *Server {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	base, err := parseBaseURL(o.baseURL)
	if err != nil {
		t.Fatalf("githubtest: %v", err)
	}
	s := &Server{repos: map[string]*Repo{}, tokens: map[string]bool{},
		apps: map[string]string{}, users: map[string]*User{}, codes: map[string]authCode{}, access: map[string]*User{},
		issuedBy: map[string]string{}, refresh: map[string]refreshGrant{}, devices: map[string]*device{},
		deviceInterval: time.Second, lfs: lfsState{objects: map[string][]byte{}}}
	s.Server = httptest.NewUnstartedServer(http.HandlerFunc(s.serve))
	if o.listener != nil {
		s.Server.Listener.Close()
		s.Server.Listener = o.listener
	}
	s.Server.Start()
	s.listenURL = s.Server.URL
	if base != "" {
		s.Server.URL = base
	}
	t.Cleanup(s.Close)
	return s
}

// ListenURL is the address the fake listens on, as "http://host:port". It equals URL unless
// WithBaseURL set another origin.
func (s *Server) ListenURL() string { return s.listenURL }

// parseBaseURL checks a WithBaseURL origin and drops a trailing slash. "" means no base URL.
func parseBaseURL(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("base URL %q: %v", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("base URL %q: scheme must be http or https", raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("base URL %q has no host", raw)
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", fmt.Errorf("base URL %q must be only a scheme, host and port", raw)
	}
	return u.Scheme + "://" + u.Host, nil
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
		branches: map[string]string{}, commits: map[string]*commit{}, extra: map[string][]TarEntry{}, tags: map[string]tag{}}
	s.mu.Lock()
	s.repos[strings.ToLower(owner+"/"+name)] = r
	s.mu.Unlock()
	r.Commit("main", files, nil)
	return r
}

// Grant lets each of tokens read the repo, and accepts each one as AddToken does. Once a private
// repo has a grant, it answers only granted tokens, and gives any other token 404, accepted or
// not, as GitHub does for a repo the token can't see. A private repo with no grants answers any
// accepted token. A public repo answers everyone either way.
func (r *Repo) Grant(tokens ...string) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.grants == nil {
		r.grants = map[string]bool{}
	}
	for _, tok := range tokens {
		r.grants[tok] = true
		r.s.tokens[tok] = true
	}
}

// GrantUser lets each of logins read the repo through any token they get by signing in, so the
// grant holds across refreshes. It limits a private repo as Grant does, and the two add up: a
// repo with grants of either kind answers a granted token or a granted person's token.
func (r *Repo) GrantUser(logins ...string) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.userGrants == nil {
		r.userGrants = map[string]bool{}
	}
	for _, l := range logins {
		r.userGrants[strings.ToLower(l)] = true
	}
}

// visible reports whether a request with tok, accepted or not, can see r.
func (s *Server) visible(r *Repo, tok string, accepted bool) bool {
	if r == nil {
		return false
	}
	if !r.Private {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return accepted && allows(r.grants, r.userGrants, tok, s.userOf(tok))
}

// allows reports whether tok, held by u or by no one, passes a limit of tokens and logins. No
// limit at all passes everyone. Callers hold s.mu.
func allows(tokens, logins map[string]bool, tok string, u *User) bool {
	if len(tokens) == 0 && len(logins) == 0 {
		return true
	}
	return tokens[tok] || u != nil && logins[strings.ToLower(u.Login)]
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

// Tag points the tag name at commit sha, replacing any tag of that name. An annotated tag is served
// as GitHub serves one, through a tag object. It panics on a sha the repo doesn't have.
func (r *Repo) Tag(name, sha string, annotated bool) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.commits[sha] == nil {
		panic("githubtest: no commit " + sha)
	}
	r.tags[name] = tag{sha: sha, annotated: annotated}
}

// SetCommitTime sets commit sha's date, so a test can make the order of tags by date differ from
// their order by name. It panics on a sha the repo doesn't have.
func (r *Repo) SetCommitTime(sha string, t time.Time) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	c := r.commits[sha]
	if c == nil {
		panic("githubtest: no commit " + sha)
	}
	c.date = t.UTC()
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
	r.commits[sha] = &commit{sha: sha, parent: r.branches[branch], files: files,
		message: fmt.Sprintf("commit %d on %s\n\nmade by githubtest", r.seq, branch),
		date:    epoch.Add(time.Duration(r.seq-1) * time.Minute)}
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
	// Git, and the LFS API, send a token as the basic-auth password.
	if _, pw, ok := req.BasicAuth(); ok {
		return pw
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
	accepted := tok != "" && (s.tokens[tok] || s.access[tok] != nil || s.accept != nil && s.accept(tok))
	s.mu.Unlock()

	if fail != 0 {
		if fail == http.StatusForbidden {
			w.Header().Set("x-ratelimit-remaining", "0")
		}
		w.WriteHeader(fail)
		return
	}
	// The sign-in endpoints take client credentials, never a bearer token.
	if strings.HasPrefix(req.URL.Path, "/login/") {
		s.serveSignIn(w, req)
		return
	}
	if tok != "" && !accepted {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "Bad credentials"})
		return
	}
	// A GitHub Enterprise Server host serves its API under /api/v3, so a client pointed at the
	// fake as one reaches the same routes.
	p := req.URL.Path
	if rest, ok := strings.CutPrefix(p, "/api/v3/"); ok {
		p = "/" + rest
	}
	if p == "/user" && req.Method == http.MethodGet {
		s.serveUser(w, tok)
		return
	}
	if org, ok := strings.CutPrefix(p, "/orgs/"); ok && req.Method == http.MethodGet {
		if o, login, ok := strings.Cut(org, "/members/"); ok && !strings.Contains(login, "/") {
			s.serveMembership(w, o, login, tok, accepted)
			return
		}
	}

	if p == "/graphql" && req.Method == http.MethodPost {
		s.graphql(w, req, tok, accepted)
		return
	}
	if raw, ok := strings.CutPrefix(p, "/raw/"); ok && req.Method == http.MethodGet {
		s.raw(w, raw, tok, accepted)
		return
	}
	if strings.HasSuffix(p, ".git/info/lfs/objects/batch") && req.Method == http.MethodPost {
		s.lfsBatch(w, req, p, tok, accepted)
		return
	}
	if oid, ok := strings.CutPrefix(p, "/lfs-objects/"); ok && req.Method == http.MethodGet {
		s.lfsObject(w, req, oid)
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(p, "/repos/"), "/", 4)
	if !strings.HasPrefix(p, "/repos/") || len(parts) < 2 {
		notFound(w)
		return
	}
	s.mu.Lock()
	r := s.repos[strings.ToLower(parts[0]+"/"+parts[1])]
	s.mu.Unlock()
	if !s.visible(r, tok, accepted) {
		notFound(w)
		return
	}
	switch {
	case len(parts) == 2 && req.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"full_name": r.Owner + "/" + r.Name, "private": r.Private, "default_branch": r.defaultBranch})
	case len(parts) == 3 && parts[2] == "commits" && req.Method == http.MethodGet:
		r.listCommits(w, req)
	case len(parts) == 3 && parts[2] == "tags" && req.Method == http.MethodGet:
		r.listTags(w, req)
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
	case len(parts) >= 3 && parts[2] == "hooks":
		r.serveHooks(w, req, parts, tok, accepted)
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

// raw serves "<owner>/<repo>/<commit>/<path>". A private repo answers 404 to a token that can't see
// it, as GitHub does.
func (s *Server) raw(w http.ResponseWriter, p, tok string, accepted bool) {
	parts := strings.SplitN(p, "/", 4)
	if len(parts) < 4 {
		notFound(w)
		return
	}
	s.mu.Lock()
	r := s.repos[strings.ToLower(parts[0]+"/"+parts[1])]
	s.mu.Unlock()
	if !s.visible(r, tok, accepted) {
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
// resolve finds the commit ref names, as GitHub does wherever a ref goes: a branch first, then a
// tag, then a commit SHA. An empty ref is the default branch.
func (r *Repo) resolve(ref string) *commit {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if ref == "" {
		ref = r.defaultBranch
	}
	if sha, ok := r.branches[ref]; ok {
		return r.commits[sha]
	}
	if t, ok := r.tags[ref]; ok {
		return r.commits[t.sha]
	}
	return r.commits[ref]
}

func (r *Repo) getCommit(w http.ResponseWriter, ref string) {
	c := r.resolve(ref)
	if c == nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"message": "No commit found for SHA: " + ref})
		return
	}
	writeJSON(w, http.StatusOK, commitJSON(c))
}

// commitJSON is a commit as GitHub's commits API shows it, with the fields the driver reads.
func commitJSON(c *commit) map[string]any {
	return map[string]any{"sha": c.sha, "commit": map[string]any{
		"message":   c.message,
		"author":    map[string]any{"name": "githubtest", "date": c.date.Format(time.RFC3339)},
		"committer": map[string]any{"name": "githubtest", "date": c.date.Format(time.RFC3339)},
	}}
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

// listCommits answers GET /repos/{o}/{r}/commits?sha=<branch or commit>, newest first, following
// parents, with per_page and page and a Link header naming the next page, as GitHub pages.
func (r *Repo) listCommits(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	c := r.resolve(q.Get("sha"))
	if c == nil {
		notFound(w)
		return
	}
	per, _ := strconv.Atoi(q.Get("per_page"))
	if per <= 0 {
		per = 30
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page <= 0 {
		page = 1
	}
	r.s.mu.Lock()
	var all []map[string]any
	for ; c != nil; c = r.commits[c.parent] {
		all = append(all, commitJSON(c))
	}
	r.s.mu.Unlock()
	r.writePage(w, req, all, page, per)
}

// listTags answers GET /repos/{o}/{r}/tags, with per_page, page and a Link header as GitHub pages.
// It orders tags by version, newest first, as GitHub does, so v1.10 comes before v1.9. Each
// tag's commit is the one it points at, annotated or not.
func (r *Repo) listTags(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	per, _ := strconv.Atoi(q.Get("per_page"))
	if per <= 0 {
		per = 30
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page <= 0 {
		page = 1
	}
	r.s.mu.Lock()
	names := make([]string, 0, len(r.tags))
	for name := range r.tags {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return versionLess(names[j], names[i]) })
	all := make([]map[string]any, len(names))
	for i, name := range names {
		all[i] = map[string]any{"name": name, "commit": map[string]any{"sha": r.tags[name].sha}}
	}
	r.s.mu.Unlock()
	r.writePage(w, req, all, page, per)
}

// versionLess compares two tag names by their runs of digits and non-digits, reading each run of
// digits as a number, so v1.9 sorts before v1.10.
func versionLess(a, b string) bool {
	for a != "" && b != "" {
		ra, rb := run(a), run(b)
		a, b = a[len(ra):], b[len(rb):]
		if ra == rb {
			continue
		}
		if isDigit(ra[0]) && isDigit(rb[0]) {
			na, nb := strings.TrimLeft(ra, "0"), strings.TrimLeft(rb, "0")
			if len(na) != len(nb) {
				return len(na) < len(nb)
			}
			if na != nb {
				return na < nb
			}
			continue
		}
		return ra < rb
	}
	return len(a) < len(b)
}

// run returns s's first run of digits, or of other bytes.
func run(s string) string {
	i := 1
	for i < len(s) && isDigit(s[i]) == isDigit(s[0]) {
		i++
	}
	return s[:i]
}

func isDigit(c byte) bool { return '0' <= c && c <= '9' }

// writePage writes page page of all, per to a page, with a Link header naming the next page.
func (r *Repo) writePage(w http.ResponseWriter, req *http.Request, all []map[string]any, page, per int) {
	q := req.URL.Query()
	lo := min((page-1)*per, len(all))
	hi := min(lo+per, len(all))
	if hi < len(all) {
		next := *req.URL
		q.Set("page", strconv.Itoa(page+1))
		next.RawQuery = q.Encode()
		w.Header().Set("Link", fmt.Sprintf(`<%s%s>; rel="next"`, r.s.URL, next.String()))
	}
	writeJSON(w, http.StatusOK, all[lo:hi])
}

// graphql answers the one GraphQL query the driver sends, a repository's tag refs. It reads the
// variables owner, name, first and after, and doesn't parse the query, except to check that it
// asks for TAG_COMMIT_DATE order, which it orders by as GitHub does. Without that it lists tags by
// name, as GitHub does by default.
// GitHub's GraphQL API refuses a request with no token, and so does the fake.
func (s *Server) graphql(w http.ResponseWriter, req *http.Request, tok string, accepted bool) {
	if !accepted {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"message": "This endpoint requires you to be authenticated."})
		return
	}
	var body struct {
		Query     string `json:"query"`
		Variables struct {
			Owner string  `json:"owner"`
			Name  string  `json:"name"`
			First int     `json:"first"`
			After *string `json:"after"`
		} `json:"variables"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Problems parsing JSON"})
		return
	}
	v := body.Variables
	s.mu.Lock()
	r := s.repos[strings.ToLower(v.Owner+"/"+v.Name)]
	s.mu.Unlock()
	if !s.visible(r, tok, accepted) {
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"repository": nil},
			"errors": []map[string]any{{"type": "NOT_FOUND", "message": "Could not resolve to a Repository with the name '" + v.Owner + "/" + v.Name + "'."}}})
		return
	}
	type node struct {
		name string
		t    tag
		c    *commit
	}
	r.s.mu.Lock()
	var nodes []node
	for name, t := range r.tags {
		nodes = append(nodes, node{name, t, r.commits[t.sha]})
	}
	r.s.mu.Unlock()
	if strings.Contains(body.Query, "TAG_COMMIT_DATE") {
		// GitHub orders by the commit's day, then by name descending, so two tags on one day can
		// come back in the wrong order by time. golang/go's go1.27.3 and go1.26.10 did in 2026-11.
		sort.Slice(nodes, func(i, j int) bool {
			di, dj := nodes[i].c.date.Truncate(24*time.Hour), nodes[j].c.date.Truncate(24*time.Hour)
			if !di.Equal(dj) {
				return di.After(dj)
			}
			return nodes[i].name > nodes[j].name
		})
	} else {
		sort.Slice(nodes, func(i, j int) bool { return nodes[i].name < nodes[j].name })
	}
	start := 0
	if v.After != nil {
		start, _ = strconv.Atoi(*v.After)
	}
	first := v.First
	if first <= 0 || first > 100 {
		first = 100
	}
	start = min(start, len(nodes))
	end := min(start+first, len(nodes))
	var out []map[string]any
	for _, n := range nodes[start:end] {
		headline, _, _ := strings.Cut(n.c.message, "\n")
		target := map[string]any{"__typename": "Commit", "oid": n.c.sha, "committedDate": n.c.date.Format(time.RFC3339), "messageHeadline": headline}
		if n.t.annotated {
			target = map[string]any{"__typename": "Tag", "target": target}
		}
		out = append(out, map[string]any{"name": n.name, "target": target})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"repository": map[string]any{"refs": map[string]any{
		"pageInfo": map[string]any{"hasNextPage": end < len(nodes), "endCursor": strconv.Itoa(end)},
		"nodes":    out,
	}}}})
}

// SignedRequest builds a webhook delivery to target as GitHub sends one: a POST of body with the
// event name in X-GitHub-Event and X-Hub-Signature-256 set to the HMAC-SHA256 of body under secret.
// Serve it to an app's handler with httptest.NewRecorder, or send it with http.DefaultClient.
func SignedRequest(target string, secret []byte, event string, body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(string(body)))
	req.RequestURI = ""
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", event)
	sum := sha1.Sum(body)
	req.Header.Set("X-GitHub-Delivery", hex.EncodeToString(sum[:16]))
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	return req
}

// PushRequest builds a signed push event to target for branch's head, with its parent as before,
// as GitHub sends after a push of one commit. It panics on a missing branch.
func (r *Repo) PushRequest(target string, secret []byte, branch string) *http.Request {
	r.s.mu.Lock()
	head, ok := r.branches[branch]
	var before string
	if ok {
		before = r.commits[head].parent
	}
	def := r.defaultBranch
	r.s.mu.Unlock()
	if !ok {
		panic("githubtest: no branch " + branch)
	}
	if before == "" {
		before = strings.Repeat("0", 40)
	}
	body, _ := json.Marshal(map[string]any{
		"ref": "refs/heads/" + branch, "before": before, "after": head,
		"created": false, "deleted": false, "forced": false,
		"repository": map[string]any{
			"name": r.Name, "full_name": r.Owner + "/" + r.Name, "private": r.Private,
			"default_branch": def, "owner": map[string]any{"login": r.Owner, "name": r.Owner},
		},
	})
	return SignedRequest(target, secret, "push", body)
}
