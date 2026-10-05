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

func TestAddKeepsWhatTheMountHasAndOverwritesARepeatedPath(t *testing.T) {
	h := New("x")
	h.Handle(catHandler(h.Root()))
	h.Mount("d", map[string][]byte{"a.txt": []byte("a1"), "b.txt": []byte("b")})
	if err := h.Add("d", map[string][]byte{"a.txt": []byte("a2"), "sub/c.txt": []byte("c")}); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"/d/a.txt": "a2", "/d/b.txt": "b", "/d/sub/c.txt": "c"} {
		if got := get(t, h, path).Body; string(got) != want {
			t.Errorf("%s = %q, want %q", path, got, want)
		}
	}
}

func TestAddCreatesAMissingMount(t *testing.T) {
	h := New("x")
	h.Handle(catHandler(h.Root()))
	if err := h.Add("new", map[string][]byte{"f": []byte("x")}); err != nil {
		t.Fatal(err)
	}
	if got := get(t, h, "/new/f").Body; string(got) != "x" {
		t.Errorf("got %q", got)
	}
}

func TestAFailedAddLeavesTheMountUnchanged(t *testing.T) {
	h := New("x")
	h.Handle(catHandler(h.Root()))
	h.Mount("d", map[string][]byte{"file": []byte("f"), "dir/x": []byte("x")})
	bad := []map[string][]byte{
		{"ok.txt": []byte("1"), "dir": []byte("over a directory")},
		{"ok.txt": []byte("1"), "file/under": []byte("under a file")},
		{"ok.txt": []byte("1"), "p": nil, "p/q": nil},
		{"ok.txt": []byte("1"), "../escape": nil},
	}
	for _, files := range bad {
		if err := h.Add("d", files); err == nil {
			t.Errorf("Add(%v) succeeded", files)
		}
	}
	if res := get(t, h, "/d/ok.txt"); res.Status != http.StatusNotFound {
		t.Errorf("a failed Add left ok.txt behind: %d %q", res.Status, res.Body)
	}
	if got := get(t, h, "/d/file").Body; string(got) != "f" {
		t.Errorf("file = %q", got)
	}
	if err := h.Add("bad/name", map[string][]byte{"f": nil}); err == nil {
		t.Error("Add accepted a mount name with a slash")
	}
}

func TestAddRebuildsOncePerBatch(t *testing.T) {
	h := New("x")
	builds := 0
	h.Rebuild(func(root fs.FS) (http.Handler, error) {
		builds++
		return catHandler(root), nil
	})
	h.Mount("d", map[string][]byte{"a": []byte("a")})
	before := builds
	if err := h.Add("d", map[string][]byte{"b": []byte("b"), "c": []byte("c"), "e": []byte("e")}); err != nil {
		t.Fatal(err)
	}
	if builds-before != 1 {
		t.Errorf("Add of three files rebuilt %d times", builds-before)
	}
	if got := get(t, h, "/d/a").Body; string(got) != "a" {
		t.Errorf("rebuilt handler lost d/a: %q", got)
	}
}

// Run with -race: adds, remounts and requests interleave.
func TestConcurrentAddsAndRequests(t *testing.T) {
	h := New("x")
	h.Handle(catHandler(h.Root()))
	h.Mount("d", map[string][]byte{"fixed": []byte("x")})
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			h.Add("d", map[string][]byte{fmt.Sprintf("f%d", i%10): []byte("y")})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			h.Add("other", map[string][]byte{"g": []byte("z")})
			h.Unmount("other")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			if res := get(t, h, "/d/fixed"); string(res.Body) != "x" {
				t.Errorf("d/fixed = %q", res.Body)
				return
			}
		}
	}()
	wg.Wait()
}

func TestDoStreamSendsEachFlushAsItHappens(t *testing.T) {
	h := New("stream")
	var during []string
	var chunks []Chunk
	h.Handle(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusAccepted)
		for i := 0; i < 3; i++ {
			fmt.Fprintf(w, "line %d\n", i)
			w.(http.Flusher).Flush()
			during = append(during, fmt.Sprint(len(chunks)))
		}
		w.Write([]byte("tail"))
	}))
	res, err := h.DoStream(context.Background(), Request{Method: "GET", URL: "/"}, func(c Chunk) { chunks = append(chunks, c) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(during, ",") != "1,2,3" {
		t.Fatalf("chunks seen by the handler after each flush: %v; want each flush delivered before it returns", during)
	}
	if chunks[0].Status != http.StatusAccepted || chunks[0].Header.Get("Content-Type") != "application/x-ndjson" || chunks[1].Status != 0 {
		t.Fatalf("first chunk %+v, second %+v; want status and header on the first only", chunks[0], chunks[1])
	}
	var got []string
	for _, c := range chunks {
		got = append(got, string(c.Body))
	}
	if strings.Join(got, "|") != "line 0\n|line 1\n|line 2\n" || string(res.Body) != "tail" || res.Status != http.StatusAccepted {
		t.Fatalf("chunks %q, then %d %q", got, res.Status, res.Body)
	}

	whole, err := h.Do(context.Background(), Request{Method: "GET", URL: "/"})
	if err != nil || string(whole.Body) != "line 0\nline 1\nline 2\ntail" {
		t.Fatalf("Do without onChunk: %q, %v; want the whole body", whole.Body, err)
	}
}
