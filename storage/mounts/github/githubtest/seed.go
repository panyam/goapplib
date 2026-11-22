package githubtest

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Seed describes a fake GitHub Enterprise Server: people, an App, fixed tokens and repos with
// tagged commits. LoadSeed reads one from a JSON file, and Apply builds it on a fake. The JSON is:
//
//	{
//	  "app":    {"client_id": "Iv1.exercise", "client_secret": "...",
//	             "key_file": "app.pem",
//	             "permissions": {"contents": "read", "members": "read"},
//	             "installations": [{"account": "hw", "repos": ["baseboard"]}],
//	             "webhook": {"url": "https://app.test/hooks/github", "secret": "...", "events": ["push"]}},
//	  "users":  [{"login": "alice", "orgs": ["acme"]}],
//	  "tokens": ["server-token"],
//	  "repos":  [{"repo": "hw/baseboard", "private": true, "readers": ["alice"],
//	              "tokens": ["server-token"],
//	              "commits": [{"tag": "rev-b", "dir": "rev-b", "lfs": ["board.kicad_pcb"]}]}]
//	}
//
// Everything is optional except each repo's name and at least one commit. The App's
// permissions, installations and webhook model it as App describes. Without them the App only
// signs people in. Without key_file, the fake makes the App's key, and Seeded.Control's GET /
// hands it out.
//
// Each commit replaces the files on main with dir's, and tags the commit when tag is set. A
// relative dir or key_file is read beside the seed file. A commit's lfs lists paths in dir that
// go in as Git LFS pointers, with their contents kept as LFS objects and a .gitattributes line
// each. readers are the logins a repo is granted to, and tokens are fixed tokens it is granted
// to, such as a server connection's.
type Seed struct {
	App    *SeedApp   `json:"app"`
	Users  []SeedUser `json:"users"`
	Tokens []string   `json:"tokens"`
	Repos  []SeedRepo `json:"repos"`

	// Dir is where a relative dir or key_file is read from. LoadSeed sets it to the seed file's
	// folder.
	Dir string `json:"-"`
}

// SeedApp is a Seed's GitHub App.
type SeedApp struct {
	ClientID      string             `json:"client_id"`
	ClientSecret  string             `json:"client_secret"`
	KeyFile       string             `json:"key_file"`
	Permissions   map[string]string  `json:"permissions"`
	Installations []SeedInstallation `json:"installations"`
	Webhook       *SeedWebhook       `json:"webhook"`
}

// SeedInstallation installs the App on Account, on Repos (names without the owner), or on every
// repo of Account when Repos is empty.
type SeedInstallation struct {
	Account string   `json:"account"`
	Repos   []string `json:"repos"`
}

// SeedWebhook is the App's webhook. App.SetWebhook says when a push reaches it.
type SeedWebhook struct {
	URL    string   `json:"url"`
	Secret string   `json:"secret"`
	Events []string `json:"events"`
}

// SeedUser is a person who can sign in, a member of each of Orgs.
type SeedUser struct {
	Login string   `json:"login"`
	Orgs  []string `json:"orgs"`
}

// SeedRepo is a repo and its history on main.
type SeedRepo struct {
	Repo    string       `json:"repo"`
	Private bool         `json:"private"`
	Readers []string     `json:"readers"`
	Tokens  []string     `json:"tokens"`
	Commits []SeedCommit `json:"commits"`
}

// SeedCommit is one commit to main: dir's files, tagged Tag when set, with LFS's paths as LFS
// pointers.
type SeedCommit struct {
	Tag string   `json:"tag"`
	Dir string   `json:"dir"`
	LFS []string `json:"lfs"`
}

// LoadSeed reads the seed file at path. A field the format doesn't have is an error, so a typo
// doesn't pass unnoticed.
func LoadSeed(path string) (*Seed, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- a test tool reads the seed file its caller names
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var sd Seed
	if err := dec.Decode(&sd); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	sd.Dir = filepath.Dir(path)
	return &sd, nil
}

// Seeded is a fake built from a Seed. Its methods are safe to call while the fake serves.
type Seeded struct {
	Server *Server

	// App is the seed's App, or nil.
	App *App

	mu      sync.Mutex
	repos   map[string]*Repo
	files   map[string]map[string]string // each repo's files on main, so a push can delete what's gone
	commits map[string]map[string]string // repo -> tag -> sha
	users   map[string]*User
}

// Apply builds sd on s and returns it. It fails on a repo without owner/name or commits, a
// missing dir, an lfs path that isn't in its dir, or an unreadable key_file.
func (sd *Seed) Apply(s *Server) (*Seeded, error) {
	e := &Seeded{Server: s, repos: map[string]*Repo{}, files: map[string]map[string]string{},
		commits: map[string]map[string]string{}, users: map[string]*User{}}
	if a := sd.App; a != nil && a.ClientID != "" {
		e.App = s.AddApp(a.ClientID, a.ClientSecret)
		if a.KeyFile != "" {
			k, err := readKey(sd.path(a.KeyFile))
			if err != nil {
				return nil, err
			}
			e.App.SetKey(k)
		}
		if a.Permissions != nil {
			e.App.SetPermissions(a.Permissions)
		}
		for _, in := range a.Installations {
			e.App.Install(in.Account, in.Repos...)
		}
		if a.Webhook != nil {
			e.App.SetWebhook(a.Webhook.URL, a.Webhook.Secret, a.Webhook.Events...)
		}
	}
	for _, u := range sd.Users {
		e.users[strings.ToLower(u.Login)] = s.AddUser(u.Login, u.Orgs...)
	}
	for _, tok := range sd.Tokens {
		s.AddToken(tok)
	}
	for _, sr := range sd.Repos {
		owner, name, ok := strings.Cut(sr.Repo, "/")
		if !ok || len(sr.Commits) == 0 {
			return nil, fmt.Errorf("githubtest: seed repo %q wants owner/name and at least one commit", sr.Repo)
		}
		e.commits[sr.Repo] = map[string]string{}
		for _, c := range sr.Commits {
			dir := sd.path(c.Dir)
			files, err := ReadDir(dir)
			if err != nil {
				return nil, err
			}
			for _, p := range c.LFS {
				if _, ok := files[p]; !ok {
					return nil, fmt.Errorf("githubtest: seed repo %q lists %q under lfs, and %s has no such file", sr.Repo, p, dir)
				}
				// git lfs track writes a .gitattributes line per path, and a driver reads it to
				// know the repo uses LFS.
				files[".gitattributes"] += p + " filter=lfs diff=lfs merge=lfs -text\n"
			}
			sha := e.commit(sr, owner, name, files, c.LFS...)
			if c.Tag != "" {
				e.repos[sr.Repo].Tag(c.Tag, sha, false)
				e.commits[sr.Repo][c.Tag] = sha
			}
		}
	}
	return e, nil
}

func (sd *Seed) path(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(sd.Dir, p)
}

// Repo returns the seeded repo named "owner/name", or nil.
func (e *Seeded) Repo(name string) *Repo {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.repos[name]
}

// User returns the seeded person with login, or nil.
func (e *Seeded) User(login string) *User { return e.users[strings.ToLower(login)] }

// Commits returns each repo's tags and the commits they name, as {"hw/baseboard": {"rev-b": sha}}.
func (e *Seeded) Commits() map[string]map[string]string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := map[string]map[string]string{}
	for r, tags := range e.commits {
		out[r] = map[string]string{}
		for t, sha := range tags {
			out[r][t] = sha
		}
	}
	return out
}

// Replace commits dir's files to main of the seeded repo name, deleting the files the last commit
// had that dir doesn't, and returns the new commit. It delivers nothing; Repo.Deliver does.
func (e *Seeded) Replace(name, dir string) (string, error) {
	files, err := ReadDir(dir)
	if err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.repos[name] == nil {
		return "", fmt.Errorf("githubtest: no seeded repo %q", name)
	}
	owner, repo, _ := strings.Cut(name, "/")
	return e.commit(SeedRepo{Repo: name}, owner, repo, files), nil
}

// commit replaces main's files in the repo with files, making the repo on its first commit, and
// returns the new commit. Each path in lfs is then committed again as a Git LFS pointer to its
// contents, so the returned commit holds pointers there. Callers hold e.mu or own e.
func (e *Seeded) commit(sr SeedRepo, owner, name string, files map[string]string, lfs ...string) string {
	r := e.repos[sr.Repo]
	if r == nil {
		r = e.Server.AddRepo(owner, name, sr.Private, files)
		r.GrantUser(sr.Readers...)
		r.Grant(sr.Tokens...)
		e.repos[sr.Repo] = r
	} else {
		var gone []string
		for p := range e.files[sr.Repo] {
			if _, kept := files[p]; !kept {
				gone = append(gone, p)
			}
		}
		r.Commit("main", files, gone)
	}
	for _, p := range lfs {
		r.CommitLFS("main", p, files[p])
	}
	e.files[sr.Repo] = files
	return r.Head("main")
}

// Control answers requests that play GitHub's other side, for a script driving the fake from
// outside the process:
//
//	GET  /         {"web": ..., "api": ".../api/v3", "raw": ".../raw", "commits": {"hw/baseboard": {"rev-b": "<sha>"}},
//	                "app": {"id": 1, "client_id": ..., "key_pem": ..., "installations": [{"id": 1, "account": "hw", "repos": [...]}]}}
//	POST /approve  login=alice&code=ABCD-1234: alice types the device code and approves the App
//	POST /push     repo=...&branch=main&dir=...[&hook=<url>&secret=<secret>]: commit dir's files to
//	               branch, deliver the push to the repo's hooks and the App's webhook, then to hook
//
// "app" is there only when the seed has one. /push answers {"sha": "<new commit>", "status": <hook's
// HTTP status, or 0>}, or 502 when a delivery fails.
func (e *Seeded) Control() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		web := e.Server.URL
		out := map[string]any{"web": web, "api": web + "/api/v3", "raw": e.Server.RawURL(), "commits": e.Commits()}
		if a := e.App; a != nil {
			var ins []map[string]any
			for _, in := range a.Installations() {
				ins = append(ins, map[string]any{"id": in.ID, "account": in.Account, "repos": in.Repos()})
			}
			out["app"] = map[string]any{"id": a.ID, "client_id": a.ClientID, "key_pem": string(a.KeyPEM()), "installations": ins}
		}
		writeJSON(w, http.StatusOK, out)
	})
	mux.HandleFunc("POST /approve", func(w http.ResponseWriter, r *http.Request) {
		login, code := r.FormValue("login"), r.FormValue("code")
		u := e.User(login)
		if u == nil || !u.Approve(code) {
			http.Error(w, fmt.Sprintf("no pending device code %q for %q", code, login), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("POST /push", func(w http.ResponseWriter, r *http.Request) {
		repo, branch, hook := r.FormValue("repo"), r.FormValue("branch"), r.FormValue("hook")
		gr := e.Repo(repo)
		if gr == nil || branch != "main" {
			http.Error(w, fmt.Sprintf("no repo %q, or branch %q isn't main", repo, branch), http.StatusNotFound)
			return
		}
		sha, err := e.Replace(repo, r.FormValue("dir"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := gr.Deliver(branch); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		status := 0
		if hook != "" {
			resp, err := e.Server.hookClient.Do(gr.PushRequest(hook, []byte(r.FormValue("secret")), branch)) // #nosec G704 -- a test tool delivers to the hook its caller names
			if err != nil {
				http.Error(w, "delivering the push: "+err.Error(), http.StatusBadGateway)
				return
			}
			resp.Body.Close()
			status = resp.StatusCode
		}
		writeJSON(w, http.StatusOK, map[string]any{"sha": sha, "status": status})
	})
	return mux
}

// ReadDir returns every file under dir by its slash path relative to dir, for Repo.Commit or
// AddRepo. It reads through an os.Root, so a link inside dir can't reach a file outside it.
func ReadDir(dir string) (map[string]string, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	files := map[string]string{}
	err = fs.WalkDir(root.FS(), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := root.ReadFile(p)
		if err != nil {
			return err
		}
		files[p] = string(b)
		return nil
	})
	return files, err
}

// readKey reads an RSA private key in PKCS#1 or PKCS#8 PEM.
func readKey(path string) (*rsa.PrivateKey, error) {
	b, err := os.ReadFile(path) // #nosec G304 -- the key file the seed names
	if err != nil {
		return nil, err
	}
	blk, _ := pem.Decode(b)
	if blk == nil {
		return nil, fmt.Errorf("githubtest: %s holds no PEM block", path)
	}
	if k, err := x509.ParsePKCS1PrivateKey(blk.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		return nil, fmt.Errorf("githubtest: %s: %w", path, err)
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("githubtest: " + path + " is not an RSA key")
	}
	return rk, nil
}
