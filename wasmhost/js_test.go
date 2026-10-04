//go:build js && wasm

package wasmhost

import (
	"io/fs"
	"net/http"
	"strings"
	"syscall/js"
	"testing"
	"time"
)

// These run under Node (make wasm-test), the same JS host a worker gives the wasm: the exports go on
// a global, take JS values, and answer through Promises.

type settled struct {
	value js.Value
	err   string
}

// await waits for p from the test goroutine, which is not inside a js.FuncOf callback, so blocking
// here is fine and is exactly what an export must not do.
func await(t *testing.T, p js.Value) settled {
	t.Helper()
	done := make(chan settled, 1)
	onOK := js.FuncOf(func(_ js.Value, a []js.Value) any { done <- settled{value: a[0]}; return nil })
	onErr := js.FuncOf(func(_ js.Value, a []js.Value) any {
		done <- settled{err: a[0].Call("toString").String()}
		return nil
	})
	defer onOK.Release()
	defer onErr.Release()
	p.Call("then", onOK, onErr)
	select {
	case s := <-done:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("promise never settled")
		return settled{}
	}
}

func uint8(s string) js.Value {
	return bytesToJS([]byte(s))
}

func text(v js.Value) string {
	b, _ := bytesFromJS(v)
	return string(b)
}

func exportFresh(t *testing.T, h *Host) js.Value {
	t.Helper()
	js.Global().Delete(h.ns)
	release := h.Export()
	t.Cleanup(func() { release(); js.Global().Delete(h.ns) })
	return js.Global().Get(h.ns)
}

func TestMountThenHTTPReadsTheFiles(t *testing.T) {
	h := New("wasmhostTestRead")
	h.Handle(catHandler(h.Root()))
	ns := exportFresh(t, h)

	files := js.Global().Get("Object").New()
	files.Set("a/b.txt", uint8("hello from a mount"))
	if s := await(t, ns.Call("mount", "docs", files)); s.err != "" {
		t.Fatalf("mount: %s", s.err)
	}
	headers := js.Global().Get("Object").New()
	headers.Set("Accept", "text/plain")
	s := await(t, ns.Call("http", "GET", "/docs/a/b.txt", headers, js.Null()))
	if s.err != "" {
		t.Fatalf("http: %s", s.err)
	}
	if st := s.value.Get("status").Int(); st != 200 {
		t.Errorf("status %d", st)
	}
	if got := text(s.value.Get("body")); got != "hello from a mount" {
		t.Errorf("body %q", got)
	}
	if got := s.value.Get("headers").Get("X-Len").String(); got != "18" {
		t.Errorf("X-Len header %q", got)
	}

	if s := await(t, ns.Call("unmount", "docs")); s.err != "" {
		t.Fatalf("unmount: %s", s.err)
	}
	if s := await(t, ns.Call("http", "GET", "/docs/a/b.txt", js.Undefined(), js.Undefined())); s.value.Get("status").Int() != 404 {
		t.Errorf("after unmount: %v", s.value.Get("status"))
	}
}

// TestAHandlerMayWaitOnTheEventLoop is rule 1 of goapplib issue 32. The handler waits on a JS timer,
// which only fires once the event loop runs again. If http ran the handler inside its js.FuncOf
// callback instead of on a goroutine, the loop could never run and the wasm would die with "all
// goroutines are asleep - deadlock!".
func TestAHandlerMayWaitOnTheEventLoop(t *testing.T) {
	h := New("wasmhostTestLoop")
	h.Handle(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fired := make(chan struct{})
		cb := js.FuncOf(func(js.Value, []js.Value) any { close(fired); return nil })
		defer cb.Release()
		js.Global().Call("setTimeout", cb, 10)
		<-fired
		w.Write([]byte("waited"))
	}))
	ns := exportFresh(t, h)
	s := await(t, ns.Call("http", "GET", "/", js.Null(), js.Null()))
	if s.err != "" || text(s.value.Get("body")) != "waited" {
		t.Errorf("got %q, err %q", text(s.value.Get("body")), s.err)
	}
}

func TestReadyIsCalledAfterTheExportsExist(t *testing.T) {
	const name = "wasmhostTestReady"
	var sawHTTP bool
	ready := js.FuncOf(func(js.Value, []js.Value) any {
		sawHTTP = js.Global().Get(name).Get("http").Type() == js.TypeFunction
		return nil
	})
	defer ready.Release()
	obj := js.Global().Get("Object").New()
	obj.Set("ready", ready)
	js.Global().Set(name, obj)
	release := New(name).Export()
	defer release()
	defer js.Global().Delete(name)
	if !sawHTTP {
		t.Error("ready ran before http was installed, or never ran")
	}
}

func TestBadArgumentsRejectInsteadOfPanicking(t *testing.T) {
	h := New("wasmhostTestBad")
	h.Handle(catHandler(h.Root()))
	ns := exportFresh(t, h)
	files := js.Global().Get("Object").New()
	files.Set("f", "not bytes")
	cases := map[string]func() js.Value{
		"mount with a string for bytes": func() js.Value { return ns.Call("mount", "m", files) },
		"mount with no files":           func() js.Value { return ns.Call("mount", "m") },
		"http with no url":              func() js.Value { return ns.Call("http", "GET") },
		"http with a string body":       func() js.Value { return ns.Call("http", "POST", "/", js.Null(), "body") },
		"mount with a bad name":         func() js.Value { return ns.Call("mount", "a/b", js.Global().Get("Object").New()) },
	}
	// Each promise is awaited as soon as it exists: Node ends the process on a rejection that has
	// no handler yet.
	for name, call := range cases {
		if s := await(t, call()); s.err == "" {
			t.Errorf("%s: resolved", name)
		}
	}
	buf := js.Global().Get("ArrayBuffer").New(2)
	files = js.Global().Get("Object").New()
	files.Set("f", buf)
	if s := await(t, ns.Call("mount", "m", files)); s.err != "" {
		t.Errorf("mount with an ArrayBuffer: %s", s.err)
	}
}

func TestRebuildModeOverTheExports(t *testing.T) {
	h := New("wasmhostTestRebuild")
	h.Rebuild(func(root fs.FS) (http.Handler, error) {
		es, _ := fs.ReadDir(root, ".")
		var names []string
		for _, e := range es {
			names = append(names, e.Name())
		}
		mounts := strings.Join(names, ",")
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte(mounts)) }), nil
	})
	ns := exportFresh(t, h)
	await(t, ns.Call("mount", "a", js.Global().Get("Object").New()))
	await(t, ns.Call("mount", "b", js.Global().Get("Object").New()))
	s := await(t, ns.Call("http", "GET", "/", js.Null(), js.Null()))
	if got := text(s.value.Get("body")); got != "a,b" {
		t.Errorf("rebuilt handler answered %q", got)
	}
}
