//go:build js && wasm

package wasmhost

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"syscall/js"
)

// Serve answers requests with handler on globalThis[ns] and never returns. It is the whole main of
// a wasm build whose handler doesn't need the mounted files; one that does builds a Host, so it can
// pass Root to the handler, and calls Host.Serve.
func Serve(ns string, handler http.Handler) {
	New(ns).Serve(handler)
}

// ServeRebuild is Serve for a handler made over the mounted files, rebuilt on every mount change
// (see Host.Rebuild). A failed first build is logged to the console and reported to each request.
func ServeRebuild(ns string, build func(root fs.FS) (http.Handler, error)) {
	New(ns).ServeRebuild(build)
}

// Serve makes handler the host's handler, installs the exports, and never returns.
func (h *Host) Serve(handler http.Handler) {
	h.Handle(handler)
	h.Export()
	select {}
}

// ServeRebuild puts the host in rebuild mode with build, installs the exports, and never returns.
func (h *Host) ServeRebuild(build func(root fs.FS) (http.Handler, error)) {
	if err := h.Rebuild(build); err != nil {
		js.Global().Get("console").Call("error", fmt.Sprintf("wasmhost %s: %v", h.ns, err))
	}
	h.Export()
	select {}
}

// Export installs the host's functions on globalThis[ns], creating that object if it is absent, and
// then calls globalThis[ns].ready() if the loader put one there. It returns a function that removes
// the exports and releases them. Serve calls it; tests and custom mains can call it directly.
//
// The exports, all returning Promises:
//
//	http(method, url, headers, body, onChunk?, id?)
//	                                  resolves {status, headers, body: Uint8Array}. With onChunk, each
//	                                  Flush in the handler calls onChunk({status?, headers?, body})
//	                                  at once (Host.DoStream), and the final body is what came after
//	                                  the last flush. With a numeric id, cancel(id) can end the
//	                                  request's context.
//	cancel(id)                        cancels the context of request id, if it's still running; a
//	                                  handler sees it the next time it checks ctx, which a loop that
//	                                  never yields to JS never gets to do (end its worker instead)
//	mount(name, {path: Uint8Array})   resolves when the files are mounted (and, in rebuild mode, built)
//	add(name, {path: Uint8Array})     the same, but keeps the files the mount already holds (Host.Add)
//	unmount(name)                     resolves when the mount is gone
//
// Arguments are copied into Go before the call returns, so the caller may transfer or reuse its
// buffers straight away.
func (h *Host) Export() (release func()) {
	g := js.Global()
	obj := g.Get(h.ns)
	if obj.Type() != js.TypeObject {
		obj = g.Get("Object").New()
		g.Set(h.ns, obj)
	}
	fns := map[string]js.Func{
		"http":    js.FuncOf(h.jsHTTP),
		"mount":   js.FuncOf(h.jsMount),
		"add":     js.FuncOf(h.jsAdd),
		"unmount": js.FuncOf(h.jsUnmount),
		"cancel":  js.FuncOf(h.jsCancel),
	}
	for name, f := range fns {
		obj.Set(name, f)
	}
	if ready := obj.Get("ready"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}
	return func() {
		for name, f := range fns {
			obj.Delete(name)
			f.Release()
		}
	}
}

func (h *Host) jsHTTP(_ js.Value, args []js.Value) any {
	if len(args) < 2 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeString {
		return rejected(fmt.Sprintf("%s.http(method, url, headers, body)", h.ns))
	}
	req := Request{Method: args[0].String(), URL: args[1].String(), Header: http.Header{}}
	if len(args) > 2 && args[2].Type() == js.TypeObject {
		keys := js.Global().Get("Object").Call("keys", args[2])
		for i := 0; i < keys.Length(); i++ {
			k := keys.Index(i).String()
			req.Header.Set(k, args[2].Get(k).String())
		}
	}
	if len(args) > 3 && !args[3].IsNull() && !args[3].IsUndefined() {
		body, err := bytesFromJS(args[3])
		if err != nil {
			return rejected(fmt.Sprintf("%s.http body: %v", h.ns, err))
		}
		req.Body = body
	}
	var onChunk func(Chunk)
	if len(args) > 4 && args[4].Type() == js.TypeFunction {
		cb := args[4]
		onChunk = func(c Chunk) {
			out := js.Global().Get("Object").New()
			if c.Status != 0 {
				out.Set("status", c.Status)
				out.Set("headers", headersToJS(c.Header))
			}
			out.Set("body", bytesToJS(c.Body))
			cb.Invoke(out)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	var id any
	if len(args) > 5 && args[5].Type() == js.TypeNumber {
		id = args[5].Int()
		h.cancels.Store(id, cancel)
	}
	return promise(func() (js.Value, error) {
		defer func() {
			if id != nil {
				h.cancels.Delete(id)
			}
			cancel()
		}()
		res, err := h.DoStream(ctx, req, onChunk)
		if err != nil {
			return js.Value{}, err
		}
		out := js.Global().Get("Object").New()
		out.Set("status", res.Status)
		out.Set("headers", headersToJS(res.Header))
		out.Set("body", bytesToJS(res.Body))
		return out, nil
	})
}

func (h *Host) jsCancel(_ js.Value, args []js.Value) any {
	if len(args) > 0 && args[0].Type() == js.TypeNumber {
		if c, ok := h.cancels.Load(args[0].Int()); ok {
			c.(context.CancelFunc)()
		}
	}
	return js.Global().Get("Promise").Call("resolve")
}

func headersToJS(hdr http.Header) js.Value {
	out := js.Global().Get("Object").New()
	for k, vs := range hdr {
		out.Set(k, strings.Join(vs, ", "))
	}
	return out
}

func (h *Host) jsMount(_ js.Value, args []js.Value) any {
	return h.jsFiles("mount", args, h.Mount)
}

func (h *Host) jsAdd(_ js.Value, args []js.Value) any {
	return h.jsFiles("add", args, h.Add)
}

// jsFiles copies (name, {path: Uint8Array}) into Go before returning, then runs apply on a goroutine.
func (h *Host) jsFiles(op string, args []js.Value, apply func(string, map[string][]byte) error) any {
	if len(args) != 2 || args[0].Type() != js.TypeString || args[1].Type() != js.TypeObject {
		return rejected(fmt.Sprintf("%s.%s(name, {path: Uint8Array})", h.ns, op))
	}
	name, obj := args[0].String(), args[1]
	keys := js.Global().Get("Object").Call("keys", obj)
	files := make(map[string][]byte, keys.Length())
	for i := 0; i < keys.Length(); i++ {
		p := keys.Index(i).String()
		b, err := bytesFromJS(obj.Get(p))
		if err != nil {
			return rejected(fmt.Sprintf("%s.%s %q: %v", h.ns, op, p, err))
		}
		files[p] = b
	}
	return promise(func() (js.Value, error) {
		return js.Undefined(), apply(name, files)
	})
}

func (h *Host) jsUnmount(_ js.Value, args []js.Value) any {
	if len(args) != 1 || args[0].Type() != js.TypeString {
		return rejected(fmt.Sprintf("%s.unmount(name)", h.ns))
	}
	name := args[0].String()
	return promise(func() (js.Value, error) {
		return js.Undefined(), h.Unmount(name)
	})
}

// promise runs work on its own goroutine and returns a Promise of its result. The executor returns
// at once, which is the point: the calling js.FuncOf callback must not wait on work that may itself
// wait on the JS event loop.
func promise(work func() (js.Value, error)) js.Value {
	var executor js.Func
	executor = js.FuncOf(func(_ js.Value, pa []js.Value) any {
		resolve, reject := pa[0], pa[1]
		go func() {
			defer executor.Release()
			v, err := work()
			if err != nil {
				reject.Invoke(js.Global().Get("Error").New(err.Error()))
				return
			}
			resolve.Invoke(v)
		}()
		return nil
	})
	return js.Global().Get("Promise").New(executor)
}

func rejected(msg string) js.Value {
	return js.Global().Get("Promise").Call("reject", js.Global().Get("Error").New(msg))
}

// bytesFromJS copies a Uint8Array or ArrayBuffer into Go. Anything else is an error rather than
// the panic js.CopyBytesToGo would raise, since a panic in a callback ends the worker.
func bytesFromJS(v js.Value) ([]byte, error) {
	g := js.Global()
	if v.InstanceOf(g.Get("ArrayBuffer")) {
		v = g.Get("Uint8Array").New(v)
	}
	if !v.InstanceOf(g.Get("Uint8Array")) {
		return nil, fmt.Errorf("want a Uint8Array or ArrayBuffer, got %s", v.Type())
	}
	b := make([]byte, v.Get("length").Int())
	js.CopyBytesToGo(b, v)
	return b, nil
}

func bytesToJS(b []byte) js.Value {
	out := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(out, b)
	return out
}
