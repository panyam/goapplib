//go:build js && wasm

package wasmhost

import (
	"context"
	"syscall/js"
)

// BrowserStore is the Store tsappkit's worker gives the wasm, over the Origin Private File System
// (wasmhost/<ns>/ in the page origin's private storage), so a blob Put before a reload is there
// for Get after it. The worker installs it as globalThis.wasmhostStore before Go starts; without
// one (no OPFS, or a worker from tsappkit before 0.6.4) every call returns ErrNoStore.
//
// Each call waits on a JS Promise, so it must run on a goroutine, such as a handler's; ctx ends
// the wait, though the browser may still finish the write.
func BrowserStore() Store {
	return browserStore{js.Global().Get("wasmhostStore")}
}

type browserStore struct{ v js.Value }

func (s browserStore) Get(ctx context.Context, key string) ([]byte, error) {
	if err := checkKey(key); err != nil {
		return nil, err
	}
	if s.v.Type() != js.TypeObject {
		return nil, ErrNoStore
	}
	v, err := awaitJS(ctx, s.v.Call("get", key))
	if err != nil {
		return nil, err
	}
	if v.IsNull() || v.IsUndefined() {
		return nil, ErrNotFound
	}
	return bytesFromJS(v)
}

func (s browserStore) Put(ctx context.Context, key string, b []byte) error {
	if err := checkKey(key); err != nil {
		return err
	}
	if s.v.Type() != js.TypeObject {
		return ErrNoStore
	}
	_, err := awaitJS(ctx, s.v.Call("put", key, bytesToJS(b)))
	return err
}

type jsResult struct {
	v   js.Value
	err error
}

// awaitJS waits for p on the calling goroutine. Its callbacks release themselves when p settles,
// not when ctx ends, since JS calling a released function would panic the worker.
func awaitJS(ctx context.Context, p js.Value) (js.Value, error) {
	done := make(chan jsResult, 1)
	var onOK, onErr js.Func
	settle := func(r jsResult) {
		done <- r
		onOK.Release()
		onErr.Release()
	}
	onOK = js.FuncOf(func(_ js.Value, a []js.Value) any {
		settle(jsResult{v: arg(a)})
		return nil
	})
	onErr = js.FuncOf(func(_ js.Value, a []js.Value) any {
		settle(jsResult{err: jsError(arg(a))})
		return nil
	})
	js.Global().Get("Promise").Call("resolve", p).Call("then", onOK, onErr)
	select {
	case r := <-done:
		return r.v, r.err
	case <-ctx.Done():
		return js.Undefined(), ctx.Err()
	}
}

func arg(a []js.Value) js.Value {
	if len(a) == 0 {
		return js.Undefined()
	}
	return a[0]
}

type jsErr string

func (e jsErr) Error() string { return "wasmhost: store: " + string(e) }

func jsError(v js.Value) error {
	if v.Type() == js.TypeObject {
		if m := v.Get("message"); m.Type() == js.TypeString {
			return jsErr(m.String())
		}
	}
	return jsErr(js.Global().Get("String").Invoke(v).String())
}
