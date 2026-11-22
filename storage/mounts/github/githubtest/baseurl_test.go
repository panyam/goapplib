package githubtest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// containerClient stands in for an app in a container: it resolves only host, to the fake's
// listener, and can't dial anything else, such as the host's 127.0.0.1.
func containerClient(host, listen string) *http.Client {
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if h, _, _ := net.SplitHostPort(addr); h != host {
				return nil, fmt.Errorf("container can't reach %s", addr)
			}
			return (&net.Dialer{}).DialContext(ctx, network, listen)
		},
	}}
}

func send(t *testing.T, c *http.Client, method, url, body string, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func TestBaseURLIsWhatTheFakeAdvertises(t *testing.T) {
	l, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	base := fmt.Sprintf("http://fakegh.test:%d", port)
	s := New(t, WithListener(l), WithBaseURL(base+"/"))

	if s.URL != base {
		t.Errorf("URL = %q, want %q", s.URL, base)
	}
	if !strings.HasSuffix(s.ListenURL(), fmt.Sprintf(":%d", port)) {
		t.Errorf("ListenURL = %q, want the listener's address", s.ListenURL())
	}
	if !strings.HasPrefix(s.RawURL(), base+"/") {
		t.Errorf("RawURL = %q, want it under %q", s.RawURL(), base)
	}
	if s.LFSURL() != base {
		t.Errorf("LFSURL = %q, want %q", s.LFSURL(), base)
	}

	c := containerClient("fakegh.test", l.Addr().String())
	r := s.AddRepo("alice", "assets", false, map[string]string{"a.txt": "one"})
	r.Commit("main", map[string]string{"a.txt": "two"}, nil)
	_, oid := r.CommitLFS("main", "big.bin", "lfs bytes")

	// An LFS download goes from the batch answer's href alone.
	batch, _ := json.Marshal(map[string]any{"operation": "download",
		"objects": []map[string]any{{"oid": oid, "size": len("lfs bytes")}}})
	resp, body := send(t, c, http.MethodPost, base+"/alice/assets.git/info/lfs/objects/batch", string(batch),
		map[string]string{"Accept": "application/vnd.git-lfs+json", "Content-Type": "application/vnd.git-lfs+json"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("batch = %d: %s", resp.StatusCode, body)
	}
	var out struct {
		Objects []struct {
			Actions struct {
				Download struct {
					Href   string            `json:"href"`
					Header map[string]string `json:"header"`
				} `json:"download"`
			} `json:"actions"`
		} `json:"objects"`
	}
	if err := json.Unmarshal([]byte(body), &out); err != nil || len(out.Objects) != 1 {
		t.Fatalf("batch answer %s: %v", body, err)
	}
	dl := out.Objects[0].Actions.Download
	if !strings.HasPrefix(dl.Href, base+"/") {
		t.Errorf("href = %q, want it under %q", dl.Href, base)
	}
	if resp, body := send(t, c, http.MethodGet, dl.Href, "", dl.Header); resp.StatusCode != http.StatusOK || body != "lfs bytes" {
		t.Errorf("following href = %d %q, want the object", resp.StatusCode, body)
	}

	// A page of commits leads to the next page through its Link header alone.
	resp, _ = send(t, c, http.MethodGet, base+"/repos/alice/assets/commits?sha=main&per_page=1", "", nil)
	m := regexp.MustCompile(`<([^>]+)>; rel="next"`).FindStringSubmatch(resp.Header.Get("Link"))
	if m == nil {
		t.Fatalf("Link = %q, want a next page", resp.Header.Get("Link"))
	}
	if !strings.HasPrefix(m[1], base+"/") {
		t.Errorf("next page = %q, want it under %q", m[1], base)
	}
	if resp, body := send(t, c, http.MethodGet, m[1], "", nil); resp.StatusCode != http.StatusOK || !strings.Contains(body, `"sha"`) {
		t.Errorf("following Link = %d %s, want page 2", resp.StatusCode, body)
	}

	// The device flow sends the person to the base URL.
	s.AddApp("app", "secret")
	_, body = send(t, c, http.MethodPost, base+"/login/device/code", "client_id=app",
		map[string]string{"Accept": "application/json", "Content-Type": "application/x-www-form-urlencoded"})
	var dev struct {
		VerificationURI string `json:"verification_uri"`
	}
	if err := json.Unmarshal([]byte(body), &dev); err != nil {
		t.Fatalf("device code answer %s: %v", body, err)
	}
	if dev.VerificationURI != base+"/login/device" {
		t.Errorf("verification_uri = %q, want %q", dev.VerificationURI, base+"/login/device")
	}

	// The test process reaches the same fake through ListenURL.
	if code, _ := get(t, http.MethodGet, s.ListenURL()+"/repos/alice/assets", "", ""); code != http.StatusOK {
		t.Errorf("repo through ListenURL = %d, want 200", code)
	}
}

func TestNewWithoutOptionsIsUnchanged(t *testing.T) {
	s := New(t)
	if !strings.HasPrefix(s.URL, "http://127.0.0.1:") {
		t.Errorf("URL = %q, want a 127.0.0.1 address", s.URL)
	}
	if s.ListenURL() != s.URL {
		t.Errorf("ListenURL = %q, want URL %q", s.ListenURL(), s.URL)
	}
}

func TestBaseURLMustBeAnOrigin(t *testing.T) {
	for raw, want := range map[string]string{
		"":                       "",
		"http://h:1":             "http://h:1",
		"https://h/":             "https://h",
		"http://h:1/api/v3":      "error",
		"http://h:1?x=1":         "error",
		"http://u:p@h:1":         "error",
		"ftp://h:1":              "error",
		"host.docker.internal:1": "error",
		"http://":                "error",
	} {
		got, err := parseBaseURL(raw)
		if err != nil {
			got = "error"
		}
		if got != want {
			t.Errorf("parseBaseURL(%q) = %q (%v), want %q", raw, got, err, want)
		}
	}
}
