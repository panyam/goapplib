package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunServesTheSeedAtTheBaseURL(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "rev-b"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "rev-b", "board.txt"), []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}
	seed := filepath.Join(dir, "seed.json")
	if err := os.WriteFile(seed, []byte(`{"tokens": ["tok"],
	  "repos": [{"repo": "hw/baseboard", "private": true, "tokens": ["tok"], "commits": [{"tag": "rev-b", "dir": "rev-b"}]}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{"--seed", seed, "--listen", fmt.Sprintf("127.0.0.1:%d", port),
			"--base-url", fmt.Sprintf("http://fakegh.test:%d", port)}, io.Discard, func(c string) { ready <- c })
	}()
	var ctl string
	select {
	case ctl = <-ready:
	case err := <-done:
		t.Fatalf("run ended before it was ready: %v", err)
	}

	resp, err := http.Get(ctl + "/")
	if err != nil {
		t.Fatal(err)
	}
	var info struct {
		API     string                       `json:"api"`
		Commits map[string]map[string]string `json:"commits"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&info)
	resp.Body.Close()
	if want := fmt.Sprintf("http://fakegh.test:%d/api/v3", port); info.API != want || info.Commits["hw/baseboard"]["rev-b"] == "" {
		t.Errorf("GET / = %+v, want api %s and rev-b's commit", info, want)
	}

	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/v3/repos/hw/baseboard/contents/board.txt", port), nil)
	req.Header.Set("Authorization", "Bearer tok")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(b), `"board.txt"`) {
		t.Errorf("board.txt through the listener = %d %s, want 200", resp.StatusCode, b)
	}

	cancel()
	if err := <-done; err != nil {
		t.Errorf("run after cancel = %v, want nil", err)
	}
}

func TestRunWantsASeed(t *testing.T) {
	if err := run(context.Background(), nil, io.Discard, nil); err == nil || !strings.Contains(err.Error(), "--seed") {
		t.Errorf("run with no --seed = %v, want an error naming it", err)
	}
	if err := run(context.Background(), []string{"--seed", filepath.Join(t.TempDir(), "x.json")}, io.Discard, nil); err == nil {
		t.Error("run with a missing seed file = nil, want an error")
	}
	seed := filepath.Join(t.TempDir(), "seed.json")
	if err := os.WriteFile(seed, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), []string{"--seed", seed, "--base-url", "http://h/api/v3"}, io.Discard, nil); err == nil || !strings.Contains(err.Error(), "base URL") {
		t.Errorf("run with a base URL that has a path = %v, want the base URL error", err)
	}
}
