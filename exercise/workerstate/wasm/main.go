//go:build js && wasm

// The worker side of the worker-state exercise: the toy service as wasm, behind wasmhost.
package main

import (
	"github.com/panyam/goapplib/exercise/workerstate/service"
	"github.com/panyam/goapplib/wasmhost"
)

func main() {
	wasmhost.Serve("state", (&service.Service{}).Handler())
}
