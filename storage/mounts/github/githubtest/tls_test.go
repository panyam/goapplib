package githubtest

import (
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestWithTLSServesHTTPS(t *testing.T) {
	s := New(t, WithTLS(nil))
	if !strings.HasPrefix(s.URL, "https://") || s.ListenURL() != s.URL {
		t.Errorf("URL = %q, ListenURL = %q; want one https address", s.URL, s.ListenURL())
	}
	s.AddRepo("alice", "assets", false, map[string]string{"a.txt": "one"})

	resp, body := send(t, s.Client(), http.MethodGet, s.URL+"/api/v3/repos/alice/assets/contents/a.txt", "", nil)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, `"a.txt"`) {
		t.Errorf("a.txt over HTTPS = %d %s, want 200", resp.StatusCode, body)
	}
	if resp, err := http.Get("http://" + strings.TrimPrefix(s.URL, "https://") + "/api/v3/repos/alice/assets"); err == nil {
		if resp.StatusCode == http.StatusOK {
			t.Errorf("plain HTTP to the TLS listener = 200, want it refused")
		}
		resp.Body.Close()
	}
}

func TestWithTLSKeepsTheBaseURL(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := New(t, WithListener(l), WithTLS(nil), WithBaseURL("https://fakegh.test:7071"))
	if s.URL != "https://fakegh.test:7071" {
		t.Errorf("URL = %q, want the base URL", s.URL)
	}
	if want := "https://" + l.Addr().String(); s.ListenURL() != want {
		t.Errorf("ListenURL = %q, want %q", s.ListenURL(), want)
	}
}
