//go:build js && wasm

// The exercise's wasm build: FilesService served from a Web Worker on globalThis.files.
package main

import (
	"github.com/panyam/goapplib/exercise/wasmhost/service"
	"github.com/panyam/goapplib/wasmhost"
)

func main() {
	h := wasmhost.New("files")
	h.Serve(service.Handler(h.Root()))
}
