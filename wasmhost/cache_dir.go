//go:build !(js && wasm)

package wasmhost

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// DirCache is a Cache in a directory, one file per key, for a service running natively (on a
// server, or in tests). The directory is created on the first Put.
func DirCache(dir string) Cache {
	return dirCache(dir)
}

type dirCache string

func (d dirCache) Get(_ context.Context, key string) ([]byte, error) {
	if err := checkKey(key); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(string(d), key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrMiss
	}
	return b, err
}

// Put writes a temporary file and renames it over the key, so a concurrent Get sees the old blob
// or the new one, never part of one.
func (d dirCache) Put(_ context.Context, key string, b []byte) error {
	if err := checkKey(key); err != nil {
		return err
	}
	if err := os.MkdirAll(string(d), 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(string(d), ".put-*")
	if err != nil {
		return err
	}
	_, werr := f.Write(b)
	cerr := f.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(f.Name())
		return err
	}
	if err := os.Rename(f.Name(), filepath.Join(string(d), key)); err != nil {
		os.Remove(f.Name())
		return err
	}
	return nil
}
