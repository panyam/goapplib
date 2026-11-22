// Package graphtest is a fake Microsoft Graph for tests of code that reads SharePoint and OneDrive
// drives: the msgraph mount driver, and an app's own tests. It serves the routes the driver uses
// over drives held in memory: a folder's children, paged with @odata.nextLink; an item by path or
// id; its content, through a redirect to a download URL as Graph does; its version history and a
// version's content; the drive's delta feed; and subscriptions.
//
// A drive is reachable as /drives/{id}, as /sites/{site}/drives/{id} after AddSiteDrive, and as
// /me/drive after SetMe. Every route needs a bearer token the fake accepts (AddToken, or
// AcceptToken for tokens another fake issued, such as connections/entratest's), and answers 401
// otherwise.
//
// Drive.Put changes a file as another writer would. With versions kept, the default, the file's
// old content stays in its version history. Each change moves the file's eTag and cTag and its
// last-modified time, and is recorded for the delta feed.
package graphtest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Server is the fake. Its URL plus "/v1.0" is the base a driver is pointed at.
type Server struct {
	*httptest.Server

	mu       sync.Mutex
	drives   map[string]*Drive
	sites    map[string]string // site/drive -> drive id
	me       string
	tokens   map[string]bool
	accept   func(string) bool
	subs     map[string]*Subscription
	requests []Request
	seq      int
	clock    time.Time

	// PageSize is how many children one listing page holds. It defaults to 2, so a test of a
	// small folder still follows @odata.nextLink.
	PageSize int
}

// Request is one request the fake answered.
type Request struct {
	Method string
	Path   string
	Token  string
}

// Subscription is a subscription made through the fake.
type Subscription struct {
	ID              string `json:"id"`
	Resource        string `json:"resource"`
	ChangeType      string `json:"changeType"`
	NotificationURL string `json:"notificationUrl"`
	ClientState     string `json:"clientState"`
	Expires         string `json:"expirationDateTime"`
}

// Drive is one drive in the fake.
type Drive struct {
	s        *Server
	ID       string
	items    map[string]*node // by id
	root     *node
	versions bool
	changes  []change // the delta feed, oldest first
	grants   map[string]bool
	allow    func(token string) bool
}

type node struct {
	id       string
	name     string
	parent   *node
	folder   bool
	kids     map[string]*node
	content  []byte
	rev      int
	modified time.Time
	history  []version // older versions, oldest first
	deleted  bool
}

type version struct {
	id       string
	content  []byte
	modified time.Time
}

type change struct {
	seq int
	n   *node
}

// New starts a fake and stops it when the test ends.
func New(t testing.TB) *Server {
	s := &Server{drives: map[string]*Drive{}, sites: map[string]string{}, tokens: map[string]bool{},
		subs: map[string]*Subscription{}, clock: time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC), PageSize: 2}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// BaseURL is the Graph root a driver is pointed at with msgraph.WithBaseURL.
func (s *Server) BaseURL() string { return s.URL + "/v1.0" }

// AddToken makes tok an accepted token.
func (s *Server) AddToken(tok string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokens[tok] = true
}

// AcceptToken also accepts any token f approves.
func (s *Server) AcceptToken(f func(string) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accept = f
}

// AddDrive makes an empty drive that keeps versions.
func (s *Server) AddDrive(id string) *Drive {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := &Drive{s: s, ID: id, items: map[string]*node{}, versions: true}
	d.root = &node{id: s.nextID(), folder: true, kids: map[string]*node{}, modified: s.clock}
	d.items[d.root.id] = d.root
	s.drives[id] = d
	return d
}

// AddSiteDrive makes a drive reachable as /sites/{site}/drives/{id} and /drives/{id}.
func (s *Server) AddSiteDrive(site, id string) *Drive {
	d := s.AddDrive(id)
	s.mu.Lock()
	s.sites[site+"/"+id] = id
	s.mu.Unlock()
	return d
}

// SetMe makes /me/drive the drive id.
func (s *Server) SetMe(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.me = id
}

// Requests returns every request answered so far, oldest first.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

// Subscriptions returns the live subscriptions, by ID.
func (s *Server) Subscriptions() map[string]Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]Subscription{}
	for id, sub := range s.subs {
		out[id] = *sub
	}
	return out
}

func (s *Server) nextID() string {
	s.seq++
	return fmt.Sprintf("ITEM%04d", s.seq)
}

// tick moves the fake's clock a minute, so each change has its own modified time.
func (s *Server) tick() time.Time {
	s.clock = s.clock.Add(time.Minute)
	return s.clock
}

// Grant limits the drive to tokens, and to whatever Allow approves. Once the drive has a grant or
// an Allow, any other accepted token gets 403 accessDenied on its routes, as SharePoint answers a
// person who can't open a library. A drive with neither answers every accepted token.
func (d *Drive) Grant(tokens ...string) {
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	if d.grants == nil {
		d.grants = map[string]bool{}
	}
	for _, t := range tokens {
		d.grants[t] = true
	}
}

// Allow limits the drive as Grant does, to tokens f approves, such as an entratest token issued
// to one person.
func (d *Drive) Allow(f func(token string) bool) {
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	d.allow = f
}

// opens reports whether tok may use the drive. Callers hold s.mu.
func (d *Drive) opens(tok string) bool {
	if d.grants == nil && d.allow == nil {
		return true
	}
	return d.grants[tok] || d.allow != nil && d.allow(tok)
}

// KeepVersions sets whether a change keeps the old content in the file's version history.
func (d *Drive) KeepVersions(on bool) {
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	d.versions = on
}

// Put writes content at p, making the folders on the way.
func (d *Drive) Put(p, content string) {
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	parts := strings.Split(p, "/")
	dir := d.root
	for _, name := range parts[:len(parts)-1] {
		next := dir.kids[name]
		if next == nil {
			next = &node{id: d.s.nextID(), name: name, parent: dir, folder: true, kids: map[string]*node{}, modified: d.s.clock}
			dir.kids[name] = next
			d.items[next.id] = next
		}
		dir = next
	}
	name := parts[len(parts)-1]
	n := dir.kids[name]
	now := d.s.tick()
	if n == nil {
		n = &node{id: d.s.nextID(), name: name, parent: dir}
		dir.kids[name] = n
		d.items[n.id] = n
	} else if d.versions {
		n.history = append(n.history, version{id: fmt.Sprintf("%d.0", n.rev), content: n.content, modified: n.modified})
	}
	n.content, n.modified = []byte(content), now
	n.rev++
	d.record(n)
}

// Delete removes the file at p.
func (d *Drive) Delete(p string) {
	d.s.mu.Lock()
	defer d.s.mu.Unlock()
	n := d.lookup(p)
	if n == nil {
		return
	}
	delete(n.parent.kids, n.name)
	n.deleted = true
	d.s.tick()
	d.record(n)
}

func (d *Drive) record(n *node) {
	d.s.seq++
	d.changes = append(d.changes, change{seq: d.s.seq, n: n})
}

func (d *Drive) lookup(p string) *node {
	n := d.root
	if p == "" {
		return n
	}
	for _, name := range strings.Split(p, "/") {
		if n == nil || !n.folder {
			return nil
		}
		n = n.kids[name]
	}
	return n
}

func (n *node) path() string {
	if n.parent == nil {
		return ""
	}
	if p := n.parent.path(); p != "" {
		return p + "/" + n.name
	}
	return n.name
}

func (d *Drive) json(n *node, withPath bool) map[string]any {
	out := map[string]any{"id": n.id, "name": n.name, "lastModifiedDateTime": n.modified.Format(time.RFC3339)}
	if n.deleted {
		out["deleted"] = map[string]string{"state": "deleted"}
		return out
	}
	out["eTag"] = fmt.Sprintf("\"{%s},%d\"", n.id, n.rev)
	if n.folder {
		out["folder"] = map[string]int{"childCount": len(n.kids)}
	} else {
		out["file"] = map[string]string{"mimeType": "application/octet-stream"}
		out["cTag"] = fmt.Sprintf("\"c:{%s},%d\"", n.id, n.rev)
		out["size"] = len(n.content)
	}
	if withPath && n.parent != nil {
		pp := "/drives/" + d.ID + "/root:"
		if p := n.parent.path(); p != "" {
			pp += "/" + p
		}
		out["parentReference"] = map[string]string{"driveId": d.ID, "path": pp}
	}
	return out
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	s.mu.Lock()
	s.requests = append(s.requests, Request{Method: r.Method, Path: r.URL.EscapedPath(), Token: tok})
	accepted := tok != "" && (s.tokens[tok] || s.accept != nil && s.accept(tok))
	s.mu.Unlock()

	if id, ok := strings.CutPrefix(r.URL.Path, "/download/"); ok {
		s.download(w, id, r.URL.Query().Get("rev"))
		return
	}
	if !accepted {
		graphError(w, http.StatusUnauthorized, "InvalidAuthenticationToken", "Access token is empty or invalid.")
		return
	}
	p, ok := strings.CutPrefix(r.URL.EscapedPath(), "/v1.0/")
	if !ok {
		graphError(w, http.StatusNotFound, "BadRequest", "unknown version")
		return
	}
	if rest, ok := strings.CutPrefix(p, "subscriptions"); ok {
		s.subscriptions(w, r, strings.TrimPrefix(rest, "/"))
		return
	}
	d, rest := s.drive(p)
	if d == nil {
		graphError(w, http.StatusNotFound, "itemNotFound", "The resource could not be found.")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !d.opens(tok) {
		graphError(w, http.StatusForbidden, "accessDenied", "Access denied")
		return
	}
	switch {
	case rest == "root/children":
		s.children(w, r, d, d.root)
	case rest == "root/delta":
		s.delta(w, r, d)
	case rest == "root":
		writeJSON(w, http.StatusOK, d.json(d.root, false))
	case strings.HasPrefix(rest, "root:/"):
		addr := strings.TrimPrefix(rest, "root:/")
		escaped, suffix, _ := strings.Cut(addr, ":")
		path, err := url.PathUnescape(escaped)
		n := d.lookup(path)
		if err != nil || n == nil {
			graphError(w, http.StatusNotFound, "itemNotFound", "The resource could not be found.")
			return
		}
		switch suffix {
		case "", "/":
			writeJSON(w, http.StatusOK, d.json(n, true))
		case "/children":
			s.children(w, r, d, n)
		case "/content":
			s.redirect(w, r, n, "")
		default:
			graphError(w, http.StatusBadRequest, "invalidRequest", "unknown path suffix")
		}
	case strings.HasPrefix(rest, "items/"):
		s.itemRoute(w, r, d, strings.Split(strings.TrimPrefix(rest, "items/"), "/"))
	default:
		graphError(w, http.StatusBadRequest, "invalidRequest", "unknown route "+rest)
	}
}

// drive finds the drive a path addresses and returns the rest of the path after it.
func (s *Server) drive(p string) (*Drive, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	segs := strings.SplitN(p, "/", 5)
	switch {
	case len(segs) >= 3 && segs[0] == "drives":
		return s.drives[unescape(segs[1])], strings.Join(segs[2:], "/")
	case len(segs) >= 5 && segs[0] == "sites" && segs[2] == "drives":
		id, ok := s.sites[unescape(segs[1])+"/"+unescape(segs[3])]
		if !ok {
			return nil, ""
		}
		return s.drives[id], segs[4]
	case len(segs) >= 3 && segs[0] == "me" && segs[1] == "drive" && s.me != "":
		return s.drives[s.me], strings.Join(segs[2:], "/")
	}
	return nil, ""
}

func unescape(s string) string {
	u, err := url.PathUnescape(s)
	if err != nil {
		return s
	}
	return u
}

func (s *Server) children(w http.ResponseWriter, r *http.Request, d *Drive, n *node) {
	if !n.folder {
		graphError(w, http.StatusBadRequest, "invalidRequest", "not a folder")
		return
	}
	names := make([]string, 0, len(n.kids))
	for name := range n.kids {
		names = append(names, name)
	}
	sort.Strings(names)
	skip, _ := strconv.Atoi(r.URL.Query().Get("$skiptoken"))
	size := s.PageSize
	if top, err := strconv.Atoi(r.URL.Query().Get("$top")); err == nil && top < size {
		size = top
	}
	end := min(skip+size, len(names))
	var vals []map[string]any
	for _, name := range names[skip:end] {
		vals = append(vals, d.json(n.kids[name], true))
	}
	out := map[string]any{"value": vals}
	if end < len(names) {
		q := r.URL.Query()
		q.Set("$skiptoken", strconv.Itoa(end))
		out["@odata.nextLink"] = s.URL + r.URL.EscapedPath() + "?" + q.Encode()
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) itemRoute(w http.ResponseWriter, r *http.Request, d *Drive, segs []string) {
	n := d.items[unescape(segs[0])]
	if n == nil || n.deleted {
		graphError(w, http.StatusNotFound, "itemNotFound", "The resource could not be found.")
		return
	}
	switch {
	case len(segs) == 1:
		writeJSON(w, http.StatusOK, d.json(n, true))
	case len(segs) == 2 && segs[1] == "content":
		s.redirect(w, r, n, "")
	case len(segs) == 2 && segs[1] == "versions":
		vals := []map[string]any{{"id": fmt.Sprintf("%d.0", n.rev), "lastModifiedDateTime": n.modified.Format(time.RFC3339), "size": len(n.content)}}
		for i := len(n.history) - 1; i >= 0; i-- {
			v := n.history[i]
			vals = append(vals, map[string]any{"id": v.id, "lastModifiedDateTime": v.modified.Format(time.RFC3339), "size": len(v.content)})
		}
		writeJSON(w, http.StatusOK, map[string]any{"value": vals})
	case len(segs) == 4 && segs[1] == "versions" && segs[3] == "content":
		vid := unescape(segs[2])
		if vid == fmt.Sprintf("%d.0", n.rev) {
			s.redirect(w, r, n, "")
			return
		}
		for _, v := range n.history {
			if v.id == vid {
				s.redirect(w, r, n, vid)
				return
			}
		}
		graphError(w, http.StatusNotFound, "itemNotFound", "The version could not be found.")
	default:
		graphError(w, http.StatusBadRequest, "invalidRequest", "unknown item route")
	}
}

// redirect answers a content request with 302 to an unauthenticated download URL, as Graph does.
func (s *Server) redirect(w http.ResponseWriter, r *http.Request, n *node, rev string) {
	if n.folder {
		graphError(w, http.StatusBadRequest, "notSupported", "a folder has no content")
		return
	}
	u := s.URL + "/download/" + n.id
	if rev != "" {
		u += "?rev=" + url.QueryEscape(rev)
	}
	http.Redirect(w, r, u, http.StatusFound)
}

func (s *Server) download(w http.ResponseWriter, id, rev string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range s.drives {
		n := d.items[id]
		if n == nil || n.deleted {
			continue
		}
		data := n.content
		if rev != "" {
			data = nil
			for _, v := range n.history {
				if v.id == rev {
					data = v.content
				}
			}
			if data == nil {
				break
			}
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(data)
		return
	}
	http.NotFound(w, nil)
}

// delta serves the drive's change feed. "latest" returns no items and a token for now. A number
// returns each item changed after it, once, in its current state. Anything else is 410, as Graph
// answers for a token it has forgotten. Deleted items carry no parentReference path, as on
// SharePoint and OneDrive for Business.
func (s *Server) delta(w http.ResponseWriter, r *http.Request, d *Drive) {
	tok := r.URL.Query().Get("token")
	var vals []map[string]any
	if tok != "latest" {
		since, err := strconv.Atoi(tok)
		if err != nil {
			graphError(w, http.StatusGone, "resyncRequired", "The delta token is invalid.")
			return
		}
		seen := map[string]bool{}
		for i := len(d.changes) - 1; i >= 0; i-- {
			c := d.changes[i]
			if c.seq <= since || seen[c.n.id] {
				continue
			}
			seen[c.n.id] = true
			vals = append([]map[string]any{d.json(c.n, !c.n.deleted)}, vals...)
		}
	}
	link := s.URL + r.URL.EscapedPath() + "?token=" + strconv.Itoa(s.seq)
	writeJSON(w, http.StatusOK, map[string]any{"value": vals, "@odata.deltaLink": link})
}

func (s *Server) subscriptions(w http.ResponseWriter, r *http.Request, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case id == "" && r.Method == http.MethodPost:
		var sub Subscription
		if err := json.NewDecoder(r.Body).Decode(&sub); err != nil || sub.NotificationURL == "" || sub.Resource == "" || sub.ChangeType != "updated" {
			graphError(w, http.StatusBadRequest, "InvalidRequest", "a drive subscription needs changeType updated, a resource and a notificationUrl")
			return
		}
		s.seq++
		sub.ID = fmt.Sprintf("sub-%d", s.seq)
		s.subs[sub.ID] = &sub
		writeJSON(w, http.StatusCreated, sub)
	case id != "" && r.Method == http.MethodPatch:
		sub := s.subs[id]
		if sub == nil {
			graphError(w, http.StatusNotFound, "ResourceNotFound", "no such subscription")
			return
		}
		var body struct {
			Expires string `json:"expirationDateTime"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Expires == "" {
			graphError(w, http.StatusBadRequest, "InvalidRequest", "expirationDateTime is required")
			return
		}
		sub.Expires = body.Expires
		writeJSON(w, http.StatusOK, sub)
	case id != "" && r.Method == http.MethodDelete:
		if s.subs[id] == nil {
			graphError(w, http.StatusNotFound, "ResourceNotFound", "no such subscription")
			return
		}
		delete(s.subs, id)
		w.WriteHeader(http.StatusNoContent)
	default:
		graphError(w, http.StatusMethodNotAllowed, "InvalidRequest", "")
	}
}

// NotificationBody is the JSON Graph POSTs to a subscription's notificationUrl after a change in
// its drive.
func (s *Server) NotificationBody(subID string) []byte {
	s.mu.Lock()
	sub := s.subs[subID]
	s.mu.Unlock()
	if sub == nil {
		return nil
	}
	b, _ := json.Marshal(map[string]any{"value": []map[string]any{{
		"subscriptionId": sub.ID, "clientState": sub.ClientState, "changeType": "updated",
		"resource": sub.Resource, "tenantId": "00000000-0000-0000-0000-000000000000",
	}}})
	return b
}

func graphError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
