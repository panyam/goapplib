package wasmhost

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// catHandler answers GET /<path> with that file from root, and 404 when it is missing.
func catHandler(root fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := fs.ReadFile(root, strings.TrimPrefix(r.URL.Path, "/"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("X-Len", fmt.Sprint(len(b)))
		w.Write(b)
	})
}

func get(t *testing.T, h *Host, url string) Response {
	t.Helper()
	res, err := h.Do(context.Background(), Request{Method: "GET", URL: url})
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return res
}

func TestDoBeforeAnyHandler(t *testing.T) {
	if _, err := New("x").Do(context.Background(), Request{Method: "GET", URL: "/"}); !errors.Is(err, ErrNoHandler) {
		t.Errorf("Do with no handler: %v", err)
	}
}

func TestAFixedHandlerSeesEachMountOnItsNextRead(t *testing.T) {
	h := New("x")
	h.Handle(catHandler(h.Root()))
	if res := get(t, h, "/docs/a.txt"); res.Status != http.StatusNotFound {
		t.Errorf("before mount: status %d", res.Status)
	}
	if err := h.Mount("docs", map[string][]byte{"a.txt": []byte("one")}); err != nil {
		t.Fatal(err)
	}
	res := get(t, h, "/docs/a.txt")
	if res.Status != 200 || string(res.Body) != "one" || res.Header.Get("X-Len") != "3" {
		t.Errorf("after mount: %d %q %v", res.Status, res.Body, res.Header)
	}
	if err := h.Mount("docs", map[string][]byte{"a.txt": []byte("two")}); err != nil {
		t.Fatal(err)
	}
	if res := get(t, h, "/docs/a.txt"); string(res.Body) != "two" {
		t.Errorf("remount did not replace: %q", res.Body)
	}
	if err := h.Unmount("docs"); err != nil {
		t.Fatal(err)
	}
	if res := get(t, h, "/docs/a.txt"); res.Status != http.StatusNotFound {
		t.Errorf("after unmount: status %d", res.Status)
	}
}

func TestOneMountReachesAnotherThroughRoot(t *testing.T) {
	h := New("x")
	h.Handle(catHandler(h.Root()))
	h.Mount("design", map[string][]byte{"board.txt": []byte("lib/r.sym")})
	h.Mount("lib", map[string][]byte{"r.sym": []byte("resistor")})
	ref := get(t, h, "/design/board.txt").Body
	if got := get(t, h, "/"+string(ref)).Body; string(got) != "resistor" {
		t.Errorf("following %q got %q", ref, got)
	}
}

func TestRebuildRunsOnEveryMountChange(t *testing.T) {
	h := New("x")
	var builds []string
	err := h.Rebuild(func(root fs.FS) (http.Handler, error) {
		es, _ := fs.ReadDir(root, ".")
		var names []string
		for _, e := range es {
			names = append(names, e.Name())
		}
		snapshot := strings.Join(names, ",")
		builds = append(builds, snapshot)
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, snapshot) }), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	h.Mount("a", map[string][]byte{"f": nil})
	h.Mount("b", map[string][]byte{"f": nil})
	h.Unmount("a")
	if fmt.Sprint(builds) != "[ a a,b b]" {
		t.Errorf("builds saw %q", builds)
	}
	if got := get(t, h, "/").Body; string(got) != "b" {
		t.Errorf("handler from the last build answered %q", got)
	}
}

func TestAFailedBuildFailsRequestsUntilTheNextGoodOne(t *testing.T) {
	h := New("x")
	h.Rebuild(func(root fs.FS) (http.Handler, error) {
		if _, err := fs.Stat(root, "cfg/ok"); err != nil {
			return nil, errors.New("no cfg/ok")
		}
		return catHandler(root), nil
	})
	if _, err := h.Do(context.Background(), Request{Method: "GET", URL: "/"}); err == nil || !strings.Contains(err.Error(), "no cfg/ok") {
		t.Errorf("Do after a failed build: %v", err)
	}
	if err := h.Mount("cfg", map[string][]byte{"bad": nil}); err == nil {
		t.Error("Mount did not report the failed build")
	}
	if err := h.Mount("cfg", map[string][]byte{"ok": []byte("y")}); err != nil {
		t.Fatal(err)
	}
	if got := get(t, h, "/cfg/ok").Body; string(got) != "y" {
		t.Errorf("after a good build: %q", got)
	}
	h.Handle(catHandler(h.Root()))
	h.Unmount("cfg")
	if res := get(t, h, "/cfg/ok"); res.Status != http.StatusNotFound {
		t.Errorf("Handle did not leave rebuild mode: status %d", res.Status)
	}
}

func TestDoPassesTheRequestThroughAndRecoversAPanic(t *testing.T) {
	h := New("x")
	h.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/panic" {
			panic("boom")
		}
		b := make([]byte, r.ContentLength)
		r.Body.Read(b)
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusTeapot)
		fmt.Fprintf(w, "%s %s q=%s ct=%s body=%s", r.Method, r.URL.Path, r.URL.Query().Get("q"), r.Header.Get("Content-Type"), b)
	}))
	res, err := h.Do(context.Background(), Request{
		Method: "POST", URL: "/svc/M?q=1",
		Header: http.Header{"Content-Type": {"application/json"}},
		Body:   []byte(`{"a":1}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `POST /svc/M q=1 ct=application/json body={"a":1}`
	if res.Status != http.StatusTeapot || string(res.Body) != want || res.Header.Get("Content-Type") != "text/plain" {
		t.Errorf("got %d %q %v", res.Status, res.Body, res.Header)
	}
	if _, err := h.Do(context.Background(), Request{Method: "GET", URL: "/panic"}); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("panic not reported: %v", err)
	}
}

func TestMountRejectsABadTree(t *testing.T) {
	h := New("x")
	if err := h.Mount("bad/name", nil); err == nil {
		t.Error("accepted a mount name with a slash")
	}
	if err := h.Mount("m", map[string][]byte{"a": nil, "a/b": nil}); err == nil {
		t.Error("accepted a path that is both a file and a directory")
	}
}

// Run with -race: requests read while mounts change.
func TestConcurrentMountsAndRequests(t *testing.T) {
	h := New("x")
	h.Handle(catHandler(h.Root()))
	h.Mount("fixed", map[string][]byte{"f": []byte("x")})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			h.Mount(fmt.Sprintf("m%d", i%4), map[string][]byte{"f": []byte("y")})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			if res := get(t, h, "/fixed/f"); string(res.Body) != "x" {
				t.Errorf("fixed/f = %q", res.Body)
				return
			}
		}
	}()
	wg.Wait()
}
