package service

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/panyam/goapplib/wasmhost"
)

// The same Connect request the page's generated client sends, served natively through the host the
// worker uses, so a break in the Go half shows up here before the browser exercise runs.
func TestReadFileThroughTheHost(t *testing.T) {
	h := wasmhost.New("files")
	h.Handle(Handler(h.Root()))
	if err := h.Mount("docs", map[string][]byte{"hello.txt": []byte("hello from a mounted file")}); err != nil {
		t.Fatal(err)
	}
	call := func(body string) wasmhost.Response {
		res, err := h.Do(context.Background(), wasmhost.Request{
			Method: "POST",
			URL:    "/files.v1.FilesService/ReadFile",
			Header: http.Header{"Content-Type": {"application/json"}, "Connect-Protocol-Version": {"1"}},
			Body:   []byte(body),
		})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	res := call(`{"path":"docs/hello.txt"}`)
	if res.Status != 200 || string(res.Body) != `{"content":"hello from a mounted file"}` {
		t.Errorf("ReadFile: %d %s", res.Status, res.Body)
	}
	res = call(`{"path":"docs/missing.txt"}`)
	if res.Status != http.StatusNotFound || !strings.Contains(string(res.Body), `"not_found"`) {
		t.Errorf("missing file: %d %s", res.Status, res.Body)
	}
}
