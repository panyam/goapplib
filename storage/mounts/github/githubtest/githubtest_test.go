package githubtest

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func get(t *testing.T, method, url, tok, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

// tagsRepo asks the GraphQL route for repo's tags and reports whether it answered with the repo.
func tagsRepo(t *testing.T, s *Server, tok string, r *Repo) bool {
	t.Helper()
	q, _ := json.Marshal(map[string]any{"query": "query", "variables": map[string]any{"owner": r.Owner, "name": r.Name, "first": 10}})
	code, body := get(t, http.MethodPost, s.URL+"/graphql", tok, string(q))
	if code != http.StatusOK {
		t.Fatalf("graphql status = %d: %s", code, body)
	}
	var out struct {
		Data struct {
			Repository json.RawMessage `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatal(err)
	}
	return len(out.Data.Repository) > 0 && string(out.Data.Repository) != "null"
}

func TestGrantLimitsAPrivateRepoToItsTokens(t *testing.T) {
	s := New(t)
	x := s.AddRepo("alice", "x", true, map[string]string{"a.txt": "alice"})
	y := s.AddRepo("bob", "y", true, map[string]string{"b.txt": "bob"})
	x.Grant("tok-a")
	y.Grant("tok-b")

	routes := func(r *Repo, file string) map[string]string {
		base := s.URL + "/repos/" + r.Owner + "/" + r.Name
		return map[string]string{
			"repo":     base,
			"commit":   base + "/commits/main",
			"contents": base + "/contents/" + file + "?ref=main",
			"tarball":  base + "/tarball/main",
			"raw":      s.RawURL() + "/" + r.Owner + "/" + r.Name + "/" + r.Head("main") + "/" + file,
		}
	}
	for name, url := range routes(x, "a.txt") {
		if code, body := get(t, http.MethodGet, url, "tok-a", ""); code != http.StatusOK {
			t.Errorf("%s: token A on its own repo = %d: %s", name, code, body)
		}
	}
	for name, url := range routes(y, "b.txt") {
		if code, _ := get(t, http.MethodGet, url, "tok-a", ""); code != http.StatusNotFound {
			t.Errorf("%s: token A on B's repo = %d, want 404", name, code)
		}
	}
	if !tagsRepo(t, s, "tok-a", x) {
		t.Error("graphql: token A can't see its own repo")
	}
	if tagsRepo(t, s, "tok-a", y) {
		t.Error("graphql: token A sees B's repo")
	}
}

func TestPrivateRepoWithoutGrantsAnswersAnyAcceptedToken(t *testing.T) {
	s := New(t)
	x := s.AddRepo("alice", "x", true, map[string]string{"a.txt": "alice"})
	s.AddToken("tok-any")
	if code, body := get(t, http.MethodGet, s.URL+"/repos/alice/x", "tok-any", ""); code != http.StatusOK {
		t.Errorf("repo = %d: %s", code, body)
	}
	if !tagsRepo(t, s, "tok-any", x) {
		t.Error("graphql: an accepted token can't see a private repo with no grants")
	}
}

func TestGrantLeavesAPublicRepoOpen(t *testing.T) {
	s := New(t)
	p := s.AddRepo("alice", "pub", false, map[string]string{"a.txt": "alice"})
	p.Grant("tok-a")
	s.AddToken("tok-other")
	for _, tok := range []string{"", "tok-other"} {
		if code, _ := get(t, http.MethodGet, s.URL+"/repos/alice/pub", tok, ""); code != http.StatusOK {
			t.Errorf("token %q on a public repo = %d, want 200", tok, code)
		}
	}
}
