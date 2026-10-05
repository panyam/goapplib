// Package wasmhost runs an app's HTTP services (typically Connect handlers) as WebAssembly inside a
// browser Web Worker, answering the same generated clients a server answers.
//
// A page pushes files in by name before it asks anything, and the handler reads them through one
// fs.FS (Root) whose top-level directories are the mounts. The page side lives in tsappkit
// (@panyam/tsappkit/wasmhost), which starts the worker and hands the Connect transport a fetch that
// posts to it.
//
// The rules it encodes, each with a failure behind it (goapplib issue 32):
//
//   - Never block inside a js.FuncOf callback. Every export returns a Promise and does its work on a
//     goroutine, because a callback that waits on something the JS event loop must deliver (a fetch,
//     a timer, a Promise) deadlocks the worker.
//   - Push the files in before the request, never pull them during it. fs.FS is synchronous and
//     every browser source of bytes is asynchronous, so the files go into memory first. State a
//     service builds for itself is different: it reads and writes a Cache (BrowserCache in the
//     browser, over the Origin Private File System), whose calls wait on the browser and so are
//     made from a handler's goroutine, which is allowed to wait, never from a js.FuncOf callback.
//   - Speak the app's existing wire protocol, so its generated clients don't know whether a server
//     or the worker answered.
//
// Host holds the platform-neutral part (mount table, handler, in-process requests) and builds and
// tests natively. Serve, ServeRebuild and Host.Export, which talk to JavaScript, are js && wasm only.
package wasmhost

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"sync"

	"github.com/panyam/goutils/memfs"
	"github.com/panyam/goutils/mountfs"
)

// ErrNoHandler is what Do returns before Handle or Rebuild has given the host a handler.
var ErrNoHandler = errors.New("wasmhost: no handler")

// Host is a mount table and the handler that serves requests over it.
//
// A Host is in one of two modes. After Handle it serves one fixed handler, which reads the live
// Root and so sees each mount change on its next read. After Rebuild it calls the build function
// again on every Mount and Unmount, for apps whose handler snapshots the files when it is made.
// All methods are safe for concurrent use.
type Host struct {
	ns   string
	root mountfs.FS

	// mountMu serializes Mount, Add and Unmount, so an Add's check and its writes see one mount.
	mountMu sync.Mutex
	mounts  map[string]*memfs.FS

	mu      sync.RWMutex
	handler http.Handler
	build   func(fs.FS) (http.Handler, error)
	err     error
}

// New returns an empty host whose JavaScript exports, once Export or Serve installs them, live on
// globalThis[ns].
func New(ns string) *Host {
	return &Host{ns: ns}
}

// Namespace is the global name the host's exports are installed under.
func (h *Host) Namespace() string { return h.ns }

// Root is the live mount table as one fs.FS: mount "docs" holding "a/b.txt" reads as "docs/a/b.txt".
// It is the same value on every call, so a handler can keep it.
func (h *Host) Root() fs.FS { return &h.root }

// Handle makes handler the host's one fixed handler, leaving rebuild mode if it was in it.
func (h *Host) Handle(handler http.Handler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handler, h.build, h.err = handler, nil, nil
}

// Rebuild puts the host in rebuild mode: build is called now with Root and again after every Mount
// and Unmount, and its handler serves until the next change. If build fails, requests fail with its
// error until a later build succeeds. The error from this first build is returned.
func (h *Host) Rebuild(build func(root fs.FS) (http.Handler, error)) error {
	h.mu.Lock()
	h.build = build
	h.mu.Unlock()
	return h.rebuild()
}

// Mount replaces the mount name with files, keyed by slash-separated path. It takes ownership of the
// byte slices. In rebuild mode it returns the build's error, and the mount stays in place either way.
func (h *Host) Mount(name string, files map[string][]byte) error {
	h.mountMu.Lock()
	defer h.mountMu.Unlock()
	if err := h.replace(name, files); err != nil {
		return err
	}
	return h.rebuild()
}

// Add puts files into the mount name, keeping the files it already holds; a file at a path the mount
// already has replaces it. A missing mount is created, as Mount would. It takes ownership of the byte
// slices, and in rebuild mode it rebuilds once for the whole batch.
//
// Add checks the whole batch before writing anything, so an error (an invalid path, a path that is
// already a directory, a file under a file) leaves the mount as it was. With a fixed handler (Handle),
// a request running at the same moment may see some of the batch and not the rest; in rebuild mode
// the new handler is built after the last file is in.
func (h *Host) Add(name string, files map[string][]byte) error {
	h.mountMu.Lock()
	defer h.mountMu.Unlock()
	m, ok := h.mounts[name]
	if !ok {
		if err := h.replace(name, files); err != nil {
			return err
		}
		return h.rebuild()
	}
	if _, err := memfs.New(files); err != nil {
		return err
	}
	for p := range files {
		if err := conflict(m, p); err != nil {
			return err
		}
	}
	for p, b := range files {
		if err := m.Put(p, b); err != nil {
			return err
		}
	}
	return h.rebuild()
}

// Unmount removes the mount name, which is not an error if it is absent. In rebuild mode it returns
// the build's error.
func (h *Host) Unmount(name string) error {
	h.mountMu.Lock()
	defer h.mountMu.Unlock()
	h.root.Unmount(name)
	delete(h.mounts, name)
	return h.rebuild()
}

func (h *Host) replace(name string, files map[string][]byte) error {
	m, err := memfs.New(files)
	if err != nil {
		return err
	}
	if err := h.root.Mount(name, m); err != nil {
		return err
	}
	if h.mounts == nil {
		h.mounts = map[string]*memfs.FS{}
	}
	h.mounts[name] = m
	return nil
}

// conflict reports whether the file p can't go into m: p is a directory there, or one of p's parents
// is a file. memfs.Put would refuse the same cases, but only after earlier files in the batch had
// gone in.
func conflict(m *memfs.FS, p string) error {
	if st, err := m.Stat(p); err == nil && st.IsDir() {
		return &fs.PathError{Op: "add", Path: p, Err: fs.ErrExist}
	}
	for dir := path.Dir(p); dir != "."; dir = path.Dir(dir) {
		if st, err := m.Stat(dir); err == nil && !st.IsDir() {
			return &fs.PathError{Op: "add", Path: p, Err: fs.ErrExist}
		}
	}
	return nil
}

func (h *Host) rebuild() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.build == nil {
		return nil
	}
	h.handler, h.err = h.build(&h.root)
	if h.err != nil {
		h.handler = nil
	}
	return h.err
}

// Request is one HTTP request as the worker receives it. URL is a path with an optional query
// ("/pkg.Service/Method?x=1"); the scheme and host the page used don't matter here.
type Request struct {
	Method string
	URL    string
	Header http.Header
	Body   []byte
}

// Response is the handler's answer, buffered whole.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// Do serves req in-process and returns the buffered response. An HTTP error status is a Response,
// not an error. The error is for no handler (ErrNoHandler), a failed rebuild, an unparseable
// request, or a handler panic, which is recovered so one bad request can't kill the worker.
func (h *Host) Do(ctx context.Context, req Request) (res Response, err error) {
	h.mu.RLock()
	handler, buildErr := h.handler, h.err
	h.mu.RUnlock()
	if buildErr != nil {
		return Response{}, buildErr
	}
	if handler == nil {
		return Response{}, ErrNoHandler
	}
	r, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bytes.NewReader(req.Body))
	if err != nil {
		return Response{}, err
	}
	r.RequestURI = req.URL
	r.ContentLength = int64(len(req.Body))
	for k, vs := range req.Header {
		for _, v := range vs {
			r.Header.Add(k, v)
		}
	}
	w := &recorder{header: http.Header{}}
	defer func() {
		if p := recover(); p != nil {
			res, err = Response{}, fmt.Errorf("wasmhost: handler panicked on %s %s: %v", req.Method, req.URL, p)
		}
	}()
	handler.ServeHTTP(w, r)
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return Response{Status: w.status, Header: w.header, Body: w.body.Bytes()}, nil
}

// recorder buffers a response. httptest.ResponseRecorder would do, but httptest registers a flag,
// and this runs in production builds.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *recorder) Header() http.Header { return w.header }

func (w *recorder) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *recorder) Write(b []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.body.Write(b)
}

// Flush lets streaming handlers flush; the body is buffered whole regardless.
func (w *recorder) Flush() {}
