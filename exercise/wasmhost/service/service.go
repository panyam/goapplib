// Package service is the exercise's toy FilesService. It's transport-neutral: the wasm main serves
// it from a Web Worker through wasmhost, and the test serves it natively through wasmhost.Host.Do,
// both over the same fs.FS of mounted files.
package service

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"time"

	"connectrpc.com/connect"

	filesv1 "github.com/panyam/goapplib/exercise/wasmhost/gen/go/files/v1"
	"github.com/panyam/goapplib/exercise/wasmhost/gen/go/files/v1/filesv1connect"
)

// Handler serves FilesService over root, which the wasm host fills from the page's mounts.
func Handler(root fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(filesv1connect.NewFilesServiceHandler(&files{root: root}))
	return mux
}

type files struct {
	root fs.FS
}

func (f *files) ReadFile(_ context.Context, req *connect.Request[filesv1.ReadFileRequest]) (*connect.Response[filesv1.ReadFileResponse], error) {
	b, err := fs.ReadFile(f.root, req.Msg.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&filesv1.ReadFileResponse{Content: string(b)}), nil
}

// Spin busy-loops rather than sleeping, so it holds the worker's thread the way a long check would.
func (f *files) Spin(_ context.Context, req *connect.Request[filesv1.SpinRequest]) (*connect.Response[filesv1.SpinResponse], error) {
	deadline := time.Now().Add(time.Duration(req.Msg.Ms) * time.Millisecond)
	var n int64
	for time.Now().Before(deadline) {
		n++
	}
	return connect.NewResponse(&filesv1.SpinResponse{Iterations: n}), nil
}
