//go:build js && wasm

// The wasmhost-files demo's worker: exercise/wasmhost's FilesService, served on globalThis.files.
package main

import (
	"github.com/panyam/goapplib/exercise/wasmhost/service"
	"github.com/panyam/goapplib/wasmhost"
)

func main() {
	h := wasmhost.New("files")
	h.Serve(service.Handler(h.Root()))
}
