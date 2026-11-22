package githubtest_test

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/panyam/goapplib/storage/mounts/github/githubtest"
	"github.com/panyam/oneauth/outbound"
)

func send(t *testing.T, method, url, tok, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
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

// enterprise is a fake with alice in org hw, an App, and three private repos alice is granted:
// hw/baseboard, hw/other and acme/elsewhere, plus hw/secret, granted only to bob, and the public
// acme/open.
func enterprise(t *testing.T) (*githubtest.Server, *githubtest.App) {
	t.Helper()
	s := githubtest.New(t)
	app := s.AddApp(clientID, clientSecret)
	s.AddUser("alice", "hw")
	s.AddUser("bob", "hw")
	for _, name := range []string{"hw/baseboard", "hw/other", "acme/elsewhere"} {
		owner, repo, _ := strings.Cut(name, "/")
		s.AddRepo(owner, repo, true, map[string]string{"a.txt": name}).GrantUser("alice")
	}
	s.AddRepo("hw", "secret", true, map[string]string{"a.txt": "secret"}).GrantUser("bob")
	s.AddRepo("acme", "open", false, map[string]string{"a.txt": "open"})
	s.SignInAs("alice")
	return s, app
}

func TestAnUnmodelledAppKeepsTodaysAccess(t *testing.T) {
	s, _ := enterprise(t)
	tok, _, _ := signIn(t, s)
	for _, p := range []string{"/repos/hw/baseboard", "/repos/hw/other/contents/a.txt", "/repos/acme/elsewhere"} {
		if code, body := get(t, s, p, tok.AccessToken); code != http.StatusOK {
			t.Errorf("%s = %d %s, want 200", p, code, body)
		}
	}
	if code, _ := get(t, s, "/orgs/hw/members/alice", tok.AccessToken); code != http.StatusNoContent {
		t.Errorf("membership = %d, want 204", code)
	}
}

func TestAUserTokenReadsOnlyWhereTheAppIsInstalled(t *testing.T) {
	s, app := enterprise(t)
	app.SetPermissions(map[string]string{"contents": "read"})
	app.Install("hw", "baseboard", "secret")
	tok, _, _ := signIn(t, s)
	r := s.AddRepo("hw", "later", true, map[string]string{"a.txt": "later"})
	r.GrantUser("alice")

	for _, p := range []string{"/repos/hw/baseboard", "/api/v3/repos/hw/baseboard/contents/a.txt", "/raw/hw/baseboard/main/a.txt",
		"/repos/acme/open/contents/a.txt"} {
		if code, body := get(t, s, p, tok.AccessToken); code != http.StatusOK {
			t.Errorf("%s = %d %s, want 200", p, code, body)
		}
	}
	// hw/other and hw/later aren't in the installation, acme isn't installed at all, and alice
	// can't read hw/secret.
	for _, p := range []string{"/repos/hw/other", "/raw/hw/other/main/a.txt", "/repos/hw/later", "/repos/acme/elsewhere",
		"/repos/hw/secret"} {
		if code, _ := get(t, s, p, tok.AccessToken); code != http.StatusNotFound {
			t.Errorf("%s = %d, want 404", p, code)
		}
	}

	app.Install("hw")
	if code, _ := get(t, s, "/repos/hw/later", tok.AccessToken); code != http.StatusOK {
		t.Errorf("hw/later after installing on all of hw = %d, want 200", code)
	}
}

func TestMembershipNeedsMembersRead(t *testing.T) {
	s, app := enterprise(t)
	app.SetPermissions(map[string]string{"contents": "read", "metadata": "read"})
	app.Install("hw")
	tok, _, _ := signIn(t, s)
	code, body := get(t, s, "/orgs/hw/members/alice", tok.AccessToken)
	if code != http.StatusForbidden || !strings.Contains(body, "Resource not accessible by integration") {
		t.Errorf("membership without members = %d %s, want 403 Resource not accessible by integration", code, body)
	}
	app.SetPermissions(map[string]string{"contents": "read", "members": "read"})
	if code, body := get(t, s, "/orgs/hw/members/alice", tok.AccessToken); code != http.StatusNoContent {
		t.Errorf("membership with members:read = %d %s, want 204", code, body)
	}
}

func TestContentsPermissionGatesReadsAndWrites(t *testing.T) {
	s, app := enterprise(t)
	app.SetPermissions(map[string]string{"members": "read"})
	app.Install("hw")
	tok, _, _ := signIn(t, s)
	if code, _ := get(t, s, "/repos/hw/baseboard", tok.AccessToken); code != http.StatusOK {
		t.Errorf("repo metadata without contents = %d, want 200", code)
	}
	if code, body := get(t, s, "/repos/hw/baseboard/contents/a.txt", tok.AccessToken); code != http.StatusForbidden {
		t.Errorf("contents without contents = %d %s, want 403", code, body)
	}
	if code, _ := get(t, s, "/raw/hw/baseboard/main/a.txt", tok.AccessToken); code != http.StatusNotFound {
		t.Errorf("raw without contents = %d, want 404", code)
	}

	put := `{"message":"m","content":"bmV3","branch":"main"}`
	app.SetPermissions(map[string]string{"contents": "read"})
	if code, body := send(t, http.MethodPut, s.URL+"/repos/hw/baseboard/contents/new.txt", tok.AccessToken, put); code != http.StatusForbidden {
		t.Errorf("write with contents:read = %d %s, want 403", code, body)
	}
	app.SetPermissions(map[string]string{"contents": "write"})
	if code, body := send(t, http.MethodPut, s.URL+"/repos/hw/baseboard/contents/new.txt", tok.AccessToken, put); code != http.StatusCreated {
		t.Errorf("write with contents:write = %d %s, want 201", code, body)
	}
}

// installation is oneauth's client for an installation of app on s, as a server connection uses.
func installation(t *testing.T, s *githubtest.Server, iss string, in *githubtest.Installation, app *githubtest.App) *outbound.GitHubAppInstallation {
	t.Helper()
	g, err := outbound.NewGitHubAppInstallation(iss, in.ID, app.KeyPEM())
	if err != nil {
		t.Fatal(err)
	}
	g.BaseURL = s.URL + "/api/v3"
	return g
}

func TestAnInstallationTokenReadsItsRepos(t *testing.T) {
	s, app := enterprise(t)
	app.SetPermissions(map[string]string{"contents": "read"})
	in := app.Install("hw", "baseboard", "secret")
	for _, iss := range []string{strconv.FormatInt(app.ID, 10), app.ClientID} {
		tok, err := installation(t, s, iss, in, app).Token()
		if err != nil {
			t.Fatalf("iss %s: %v", iss, err)
		}
		if !strings.HasPrefix(tok.AccessToken, "ghs_") || time.Until(tok.Expiry) < 50*time.Minute {
			t.Errorf("iss %s: token %q expiring %v, want a ghs_ token good for an hour", iss, tok.AccessToken, tok.Expiry)
		}
		// The installation reads hw/secret, though no person or token was granted it.
		for _, p := range []string{"/repos/hw/baseboard/contents/a.txt", "/repos/hw/secret/contents/a.txt", "/repos/acme/open"} {
			if code, body := get(t, s, p, tok.AccessToken); code != http.StatusOK {
				t.Errorf("iss %s: %s = %d %s, want 200", iss, p, code, body)
			}
		}
		for _, p := range []string{"/repos/hw/other", "/repos/acme/elsewhere"} {
			if code, _ := get(t, s, p, tok.AccessToken); code != http.StatusNotFound {
				t.Errorf("iss %s: %s = %d, want 404", iss, p, code)
			}
		}
		code, body := get(t, s, "/installation/repositories", tok.AccessToken)
		var list struct {
			Total int `json:"total_count"`
			Repos []struct {
				FullName string `json:"full_name"`
			} `json:"repositories"`
		}
		_ = json.Unmarshal([]byte(body), &list)
		if code != http.StatusOK || list.Total != 2 || list.Repos[0].FullName != "hw/baseboard" || list.Repos[1].FullName != "hw/secret" {
			t.Errorf("iss %s: installation repos = %d %s, want hw/baseboard and hw/secret", iss, code, body)
		}
	}
}

func TestAppJWTRefusals(t *testing.T) {
	s, app := enterprise(t)
	in := app.Install("hw")
	other := s.AddApp("Iv1.other", "x")
	otherIn := other.Install("acme")
	stranger, _ := rsa.GenerateKey(rand.Reader, 2048)
	appKey := installation(t, s, app.ClientID, in, app).Key

	mint := func(key *rsa.PrivateKey, iss string, life time.Duration) string {
		now := time.Now()
		tok, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss": iss, "iat": now.Add(-30 * time.Second).Unix(), "exp": now.Add(life).Unix()}).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return tok
	}
	id := strconv.FormatInt(app.ID, 10)
	for name, c := range map[string]struct {
		jwt  string
		inst int64
		want int
	}{
		"good":                 {mint(appKey, id, 9*time.Minute), in.ID, http.StatusCreated},
		"unknown App":          {mint(appKey, "999", 9*time.Minute), in.ID, http.StatusNotFound},
		"another key":          {mint(stranger, id, 9*time.Minute), in.ID, http.StatusUnauthorized},
		"too long":             {mint(appKey, id, 20*time.Minute), in.ID, http.StatusUnauthorized},
		"another App's":        {mint(appKey, id, 9*time.Minute), otherIn.ID, http.StatusNotFound},
		"unknown installation": {mint(appKey, id, 9*time.Minute), 4242, http.StatusNotFound},
		"not a JWT":            {"ghu_nope", in.ID, http.StatusUnauthorized},
	} {
		code, body := send(t, http.MethodPost, s.URL+"/app/installations/"+strconv.FormatInt(c.inst, 10)+"/access_tokens", c.jwt, "")
		if code != c.want {
			t.Errorf("%s: %d %s, want %d", name, code, body, c.want)
		}
	}
}

func TestPushReachesTheAppWebhookWithItsInstallation(t *testing.T) {
	var mu sync.Mutex
	var got []*http.Request
	var bodies [][]byte
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got, bodies = append(got, r), append(bodies, b)
		mu.Unlock()
	}))
	defer hook.Close()

	s, app := enterprise(t)
	in := app.Install("hw", "board")
	app.SetWebhook(hook.URL, "whsec", "push")
	elsewhere := s.AddApp("Iv1.elsewhere", "x")
	elsewhere.Install("acme")
	elsewhere.SetWebhook(hook.URL, "other", "*")

	r := s.AddRepo("hw", "pushed", true, map[string]string{"a.txt": "one"})
	if _, err := r.Push("main", map[string]string{"a.txt": "two"}, nil); err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("a push to hw/pushed, where no App is installed, reached %d App webhooks", len(got))
	}

	board := s.AddRepo("hw", "board", true, map[string]string{"a.txt": "rev-b"})
	sha, err := board.Push("main", map[string]string{"a.txt": "rev-c"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("a push to hw/baseboard reached %d App webhooks, want 1", len(got))
	}
	var ev struct {
		After        string `json:"after"`
		Installation struct {
			ID int64 `json:"id"`
		} `json:"installation"`
	}
	if err := json.Unmarshal(bodies[0], &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Installation.ID != in.ID || ev.After != sha {
		t.Errorf("delivery %s, want installation %d and after %s", bodies[0], in.ID, sha)
	}
	mac := hmac.New(sha256.New, []byte("whsec"))
	mac.Write(bodies[0])
	if want := "sha256=" + hex.EncodeToString(mac.Sum(nil)); got[0].Header.Get("X-Hub-Signature-256") != want || got[0].Header.Get("X-GitHub-Event") != "push" {
		t.Errorf("headers %v, want a push signed with the App's secret", got[0].Header)
	}
}
