package githubtest_test

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/panyam/goapplib/storage/mounts/github/githubtest"
	"github.com/panyam/oneauth/outbound"
)

const seedApp = `{"client_id": "Iv1.x", "client_secret": "s"}`

// writeSeed writes a seed with app as its App and one private repo with two tagged commits,
// granted to alice and to a server token, revC being the second commit. It returns the seed's
// path.
func writeSeed(t *testing.T, app, revC string) string {
	t.Helper()
	dir := t.TempDir()
	for rev, body := range map[string]string{"rev-b": "b", "rev-c": "c"} {
		if err := os.MkdirAll(filepath.Join(dir, rev), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rev, "board.txt"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	seed := `{"app": ` + app + `,
	  "users": [{"login": "alice", "orgs": ["acme"]}, {"login": "mallory", "orgs": ["acme"]}],
	  "tokens": ["server-token"],
	  "repos": [{"repo": "hw/baseboard", "private": true, "readers": ["alice"], "tokens": ["server-token"],
	             "commits": [{"tag": "rev-b", "dir": "rev-b"}, ` + revC + `]}]}`
	p := filepath.Join(dir, "seed.json")
	if err := os.WriteFile(p, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func seeded(t *testing.T, path string) *githubtest.Seeded {
	t.Helper()
	sd, err := githubtest.LoadSeed(path)
	if err != nil {
		t.Fatal(err)
	}
	e, err := sd.Apply(githubtest.New(t))
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func status(t *testing.T, u, token string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestTheSeedGrantsAPrivateRepoToItsTokensAlone(t *testing.T) {
	e := seeded(t, writeSeed(t, seedApp, `{"tag": "rev-c", "dir": "rev-c"}`))
	if got, _ := status(t, e.Server.URL+"/api/v3/repos/hw/baseboard", "server-token"); got != http.StatusOK {
		t.Errorf("GET /api/v3/repos/hw/baseboard with the granted token = %d, want 200", got)
	}
	if got, _ := status(t, e.Server.URL+"/api/v3/repos/hw/baseboard", "someone-else"); got != http.StatusUnauthorized {
		t.Errorf("GET a private repo with an unknown token = %d, want 401", got)
	}
	c := e.Commits()["hw/baseboard"]
	if c["rev-b"] == "" || c["rev-b"] == c["rev-c"] {
		t.Errorf("the two tags name commits %v, want two different ones", c)
	}
}

func TestTheControlAddressPushesAndApproves(t *testing.T) {
	e := seeded(t, writeSeed(t, seedApp, `{"tag": "rev-c", "dir": "rev-c"}`))
	var gotSig, gotBody string
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody, gotSig = string(b), r.Header.Get("X-Hub-Signature-256")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer hook.Close()
	ctl := httptest.NewServer(e.Control())
	defer ctl.Close()

	next := t.TempDir()
	if err := os.WriteFile(filepath.Join(next, "board.txt"), []byte("d"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := e.Repo("hw/baseboard").Head("main")
	resp, err := http.PostForm(ctl.URL+"/push", url.Values{"repo": {"hw/baseboard"}, "branch": {"main"}, "dir": {next}, "hook": {hook.URL}, "secret": {"shh"}})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Sha    string `json:"sha"`
		Status int    `json:"status"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if out.Sha == "" || out.Sha == before || out.Status != http.StatusAccepted {
		t.Fatalf("push = %+v, want a new commit and the hook's 202", out)
	}
	mac := hmac.New(sha256.New, []byte("shh"))
	mac.Write([]byte(gotBody))
	if want := "sha256=" + hex.EncodeToString(mac.Sum(nil)); gotSig != want || !strings.Contains(gotBody, out.Sha) {
		t.Errorf("the delivery's signature %q or body doesn't match the push of %s", gotSig, out.Sha)
	}
	if code, _ := status(t, e.Server.RawURL()+"/hw/baseboard/main/board.txt", "server-token"); code != http.StatusOK {
		t.Errorf("board.txt after the push = %d, want 200", code)
	}

	resp, err = http.PostForm(ctl.URL+"/approve", url.Values{"login": {"alice"}, "code": {"NOPE-0000"}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("approving a code no device flow issued = %d, want 404", resp.StatusCode)
	}
}

func TestASeedCommitKeepsItsLFSPathsAsPointers(t *testing.T) {
	e := seeded(t, writeSeed(t, seedApp, `{"tag": "rev-c", "dir": "rev-c", "lfs": ["board.txt"]}`))
	raw := func(tag string) string {
		_, b := status(t, e.Server.RawURL()+"/hw/baseboard/"+e.Commits()["hw/baseboard"][tag]+"/board.txt", "server-token")
		return b
	}
	pointer, _ := githubtest.LFSPointer("c")
	if got := raw("rev-c"); got != pointer {
		t.Errorf("board.txt at rev-c = %q; want the LFS pointer to its contents, %q", got, pointer)
	}
	if got := raw("rev-b"); got != "b" {
		t.Errorf("board.txt at rev-b = %q; want the plain file", got)
	}
	sd, err := githubtest.LoadSeed(writeSeed(t, seedApp, `{"tag": "rev-c", "dir": "rev-c", "lfs": ["nope.txt"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sd.Apply(githubtest.New(t)); err == nil || !strings.Contains(err.Error(), "nope.txt") {
		t.Errorf("a seed listing a missing file under lfs = %v; want an error naming it", err)
	}
}

func TestASeedWithAnUnknownFieldIsRefused(t *testing.T) {
	_, err := githubtest.LoadSeed(writeSeed(t, `{"client_id": "Iv1.x", "permisions": {}}`, `{"dir": "rev-c"}`))
	if err == nil || !strings.Contains(err.Error(), "permisions") {
		t.Errorf("LoadSeed with a misspelt field = %v, want an error naming it", err)
	}
}

// A seeded App is installed where the seed says, its key comes from GET / on the control address,
// and a push through the control address reaches its webhook with the installation.
func TestASeededAppIsInstalledAndHandsOutItsKey(t *testing.T) {
	var mu sync.Mutex
	var deliveries []string
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		deliveries = append(deliveries, string(b))
		mu.Unlock()
	}))
	defer hook.Close()
	app := `{"client_id": "Iv1.x", "client_secret": "s", "permissions": {"contents": "read"},
	  "installations": [{"account": "hw", "repos": ["baseboard"]}],
	  "webhook": {"url": "` + hook.URL + `", "secret": "app-secret", "events": ["push"]}}`
	e := seeded(t, writeSeed(t, app, `{"tag": "rev-c", "dir": "rev-c"}`))
	ctl := httptest.NewServer(e.Control())
	defer ctl.Close()

	_, body := status(t, ctl.URL+"/", "")
	var info struct {
		API string `json:"api"`
		App struct {
			ID            int64  `json:"id"`
			KeyPEM        string `json:"key_pem"`
			Installations []struct {
				ID      int64    `json:"id"`
				Account string   `json:"account"`
				Repos   []string `json:"repos"`
			} `json:"installations"`
		} `json:"app"`
	}
	if err := json.Unmarshal([]byte(body), &info); err != nil {
		t.Fatal(err)
	}
	if len(info.App.Installations) != 1 || info.App.Installations[0].Account != "hw" || strings.Join(info.App.Installations[0].Repos, ",") != "baseboard" {
		t.Fatalf("GET / = %s, want the App's one installation on hw/baseboard", body)
	}
	in := info.App.Installations[0]
	g, err := outbound.NewGitHubAppInstallation(e.App.ClientID, in.ID, []byte(info.App.KeyPEM))
	if err != nil {
		t.Fatal(err)
	}
	g.BaseURL = info.API
	tok, err := g.Token()
	if err != nil {
		t.Fatal(err)
	}
	if code, body := status(t, info.API+"/repos/hw/baseboard/contents/board.txt", tok.AccessToken); code != http.StatusOK {
		t.Errorf("the installation token on hw/baseboard = %d %s, want 200", code, body)
	}

	next := t.TempDir()
	if err := os.WriteFile(filepath.Join(next, "board.txt"), []byte("d"), 0o600); err != nil {
		t.Fatal(err)
	}
	resp, err := http.PostForm(ctl.URL+"/push", url.Values{"repo": {"hw/baseboard"}, "branch": {"main"}, "dir": {next}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	mu.Lock()
	defer mu.Unlock()
	if resp.StatusCode != http.StatusOK || len(deliveries) != 1 || !strings.Contains(deliveries[0], `"installation":{"id":`) {
		t.Errorf("push = %d with App deliveries %q, want one carrying the installation", resp.StatusCode, deliveries)
	}
}

func TestASeedReadsTheAppKeyFile(t *testing.T) {
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	der, _ := x509.MarshalPKCS8PrivateKey(k)
	path := writeSeed(t, `{"client_id": "Iv1.x", "key_file": "app.pem", "installations": [{"account": "hw"}]}`, `{"dir": "rev-c"}`)
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "app.pem"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	e := seeded(t, path)
	g := &outbound.GitHubAppInstallation{AppID: "Iv1.x", InstallationID: e.App.Installations()[0].ID, Key: k, BaseURL: e.Server.URL}
	if _, err := g.Token(); err != nil {
		t.Errorf("a token with the key from key_file: %v", err)
	}
}
