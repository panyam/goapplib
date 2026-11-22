package githubtest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// Hook is a webhook made on a repo through the hooks API. Token is the token that last made or
// changed it, so a test can check that the app used the right connection.
type Hook struct {
	ID          int64
	URL         string
	ContentType string
	Secret      string
	Events      []string
	Active      bool
	Token       string
}

// HookManagers limits who may manage the repo's hooks to tokens, and accepts each one as AddToken
// does. Any other token gets 404 from the hooks API, as GitHub answers a token without admin
// rights on the repo. With no managers, any accepted token that can see the repo manages its
// hooks.
func (r *Repo) HookManagers(tokens ...string) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.managers == nil {
		r.managers = map[string]bool{}
	}
	for _, tok := range tokens {
		r.managers[tok] = true
		r.s.tokens[tok] = true
	}
}

// HookManagerUsers lets each of logins manage the repo's hooks through any token they get by
// signing in. It limits who may manage hooks as HookManagers does, and the two add up.
func (r *Repo) HookManagerUsers(logins ...string) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if r.userManagers == nil {
		r.userManagers = map[string]bool{}
	}
	for _, l := range logins {
		r.userManagers[strings.ToLower(l)] = true
	}
}

// Hooks returns a copy of the repo's hooks, oldest first.
func (r *Repo) Hooks() []Hook {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	out := make([]Hook, len(r.hooks))
	for i, h := range r.hooks {
		out[i] = *h
		out[i].Events = slices.Clone(h.Events)
	}
	return out
}

// Push commits put and del on branch, as Commit does, then delivers a signed push event to each
// active hook that wants push events, and to the webhook of each App installed on the repo that
// wants them (apps.go), one at a time, before it returns. GitHub delivers later and
// in the background; delivering first lets a test check the app's state right after Push. Each
// delivery is JSON, whatever the hook's content type. Push returns the new commit, and the first
// delivery that fails or answers other than 2xx.
func (r *Repo) Push(branch string, put map[string]string, del []string) (string, error) {
	sha := r.Commit(branch, put, del)
	return sha, r.Deliver(branch)
}

// Deliver sends the push event for branch's head to the repo's hooks and to each installed App's
// webhook, as Push does after its commit. It returns the first delivery that fails or answers
// other than 2xx.
func (r *Repo) Deliver(branch string) error {
	c := r.s.hookClient
	for _, h := range r.Hooks() {
		if !h.Active || !slices.Contains(h.Events, "push") && !slices.Contains(h.Events, "*") {
			continue
		}
		resp, err := c.Do(r.PushRequest(h.URL, []byte(h.Secret), branch))
		if err != nil {
			return fmt.Errorf("githubtest: delivering hook %d: %w", h.ID, err)
		}
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("githubtest: hook %d at %s answered %d", h.ID, h.URL, resp.StatusCode)
		}
	}
	for _, d := range r.appDeliveries() {
		resp, err := c.Do(d.inst.PushRequest(r, d.hook.url, []byte(d.hook.secret), branch))
		if err != nil {
			return fmt.Errorf("githubtest: delivering to installation %d's App webhook: %w", d.inst.ID, err)
		}
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("githubtest: App webhook %s answered %d", d.hook.url, resp.StatusCode)
		}
	}
	return nil
}

// hookBody is a create or update request. Absent fields leave a hook's value as it is.
type hookBody struct {
	Active *bool    `json:"active"`
	Events []string `json:"events"`
	Config *struct {
		URL         string `json:"url"`
		ContentType string `json:"content_type"`
		Secret      string `json:"secret"`
	} `json:"config"`
}

func (h *Hook) json() map[string]any {
	cfg := map[string]any{"url": h.URL, "content_type": h.ContentType, "insecure_ssl": "0"}
	if h.Secret != "" {
		cfg["secret"] = "********"
	}
	return map[string]any{"id": h.ID, "type": "Repository", "name": "web", "active": h.Active, "events": h.Events, "config": cfg}
}

// serveHooks serves /repos/{o}/{r}/hooks and /hooks/{id}. The caller checked that the token can
// see the repo.
func (r *Repo) serveHooks(w http.ResponseWriter, req *http.Request, parts []string, tok string, accepted bool) {
	r.s.mu.Lock()
	allowed := tok != "" && accepted && allows(r.managers, r.userManagers, tok, r.s.userOf(tok))
	r.s.mu.Unlock()
	if !allowed {
		notFound(w)
		return
	}
	if len(parts) == 3 {
		switch req.Method {
		case http.MethodGet:
			r.s.mu.Lock()
			out := []map[string]any{}
			for _, h := range r.hooks {
				out = append(out, h.json())
			}
			r.s.mu.Unlock()
			writeJSON(w, http.StatusOK, out)
		case http.MethodPost:
			r.createHook(w, req, tok)
		default:
			notFound(w)
		}
		return
	}
	id, err := strconv.ParseInt(parts[3], 10, 64)
	r.s.mu.Lock()
	var h *Hook
	for _, x := range r.hooks {
		if err == nil && x.ID == id {
			h = x
		}
	}
	r.s.mu.Unlock()
	if h == nil {
		notFound(w)
		return
	}
	switch req.Method {
	case http.MethodGet:
		r.s.mu.Lock()
		out := h.json()
		r.s.mu.Unlock()
		writeJSON(w, http.StatusOK, out)
	case http.MethodPatch:
		var b hookBody
		if json.NewDecoder(req.Body).Decode(&b) != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Problems parsing JSON"})
			return
		}
		if b.Config != nil && !hookURL(b.Config.URL) {
			invalidHook(w, "url is not a valid URL")
			return
		}
		r.s.mu.Lock()
		applyHook(h, b, tok)
		out := h.json()
		r.s.mu.Unlock()
		writeJSON(w, http.StatusOK, out)
	case http.MethodDelete:
		r.s.mu.Lock()
		r.hooks = slices.DeleteFunc(r.hooks, func(x *Hook) bool { return x == h })
		r.s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		notFound(w)
	}
}

func (r *Repo) createHook(w http.ResponseWriter, req *http.Request, tok string) {
	var b hookBody
	if json.NewDecoder(req.Body).Decode(&b) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Problems parsing JSON"})
		return
	}
	if b.Config == nil || !hookURL(b.Config.URL) {
		invalidHook(w, "url is not a valid URL")
		return
	}
	r.s.mu.Lock()
	dup := slices.ContainsFunc(r.hooks, func(x *Hook) bool { return x.URL == b.Config.URL })
	var out map[string]any
	if !dup {
		r.s.hookSeq++
		h := &Hook{ID: r.s.hookSeq, Active: true, Events: []string{"push"}, ContentType: "form"}
		applyHook(h, b, tok)
		r.hooks = append(r.hooks, h)
		out = h.json()
	}
	r.s.mu.Unlock()
	if dup {
		invalidHook(w, "Hook already exists on this repository")
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func applyHook(h *Hook, b hookBody, tok string) {
	if b.Active != nil {
		h.Active = *b.Active
	}
	if b.Events != nil {
		h.Events = slices.Clone(b.Events)
	}
	if b.Config != nil {
		h.URL, h.ContentType, h.Secret = b.Config.URL, b.Config.ContentType, b.Config.Secret
		if h.ContentType == "" {
			h.ContentType = "form"
		}
	}
	h.Token = tok
}

func hookURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func invalidHook(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"message": "Validation Failed",
		"errors": []map[string]string{{"resource": "Hook", "code": "custom", "message": msg}}})
}
