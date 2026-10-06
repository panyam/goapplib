//go:build js && wasm

// The wasmhost-state demo's worker: exercise/workerstate's service, keeping its state in the
// browser's cache, served on globalThis.state.
package main

import (
	"github.com/panyam/goapplib/exercise/workerstate/service"
	"github.com/panyam/goapplib/wasmhost"
)

func main() {
	wasmhost.Serve("state", (&service.Service{Cache: wasmhost.BrowserCache()}).Handler())
}
