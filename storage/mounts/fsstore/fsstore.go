// Package fsstore keeps mount records in a folder, one YAML file per mount, named <name>.yaml.
// Several processes can share the folder: a create is exclusive across them, and each notices the
// others' creates and deletes through Generation. It is the first mounts.RecordStore backend, and
// the one agni uses for its --store folder.
package fsstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/panyam/goapplib/storage/mounts"
	"gopkg.in/yaml.v3"
)

const ext = ".yaml"

// Store is a mounts.RecordStore over one folder. Its methods are safe to call from several
// goroutines and several processes at once.
type Store struct{ dir string }

// New returns a store over dir, creating the folder if it is missing.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// record is the YAML form of a mounts.Mount. The tags are the keys on disk, so renaming a Go field
// doesn't change the files.
type record struct {
	Name       string    `yaml:"name"`
	Kind       string    `yaml:"kind"`
	Source     string    `yaml:"source"`
	Ref        string    `yaml:"ref,omitempty"`
	Version    string    `yaml:"version,omitempty"`
	Mode       string    `yaml:"mode"`
	Connection string    `yaml:"connection,omitempty"`
	Manifest   string    `yaml:"manifest,omitempty"`
	Created    time.Time `yaml:"created"`
}

func toRecord(m mounts.Mount) record {
	return record{m.Name, string(m.Kind), m.Source, m.Ref, m.Version, string(m.Mode), m.Connection, m.Manifest, m.Created.UTC()}
}

func (r record) mount() mounts.Mount {
	return mounts.Mount{Name: r.Name, Kind: mounts.Kind(r.Kind), Source: r.Source, Ref: r.Ref, Version: r.Version,
		Mode: mounts.Mode(r.Mode), Connection: r.Connection, Manifest: r.Manifest, Created: r.Created}
}

// ListMounts reads every record, in name order. A file whose record names a different mount, or
// one that doesn't parse, fails the whole list with ErrInvalidArgument, so a hand-edited mistake
// shows up rather than hiding a mount.
func (s *Store) ListMounts(ctx context.Context) ([]mounts.Mount, error) {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}
	var out []mounts.Mount
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ext)
		if !ok || !e.Type().IsRegular() || strings.HasPrefix(name, ".") {
			continue
		}
		m, err := read(root, name)
		if errors.Is(err, mounts.ErrNotFound) {
			continue // deleted since ReadDir
		}
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// GetMount reads the record named name, or returns ErrNotFound.
func (s *Store) GetMount(ctx context.Context, name string) (mounts.Mount, error) {
	if err := mounts.ValidName(name); err != nil {
		return mounts.Mount{}, err
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return mounts.Mount{}, err
	}
	defer root.Close()
	return read(root, name)
}

func read(root *os.Root, name string) (mounts.Mount, error) {
	b, err := root.ReadFile(name + ext)
	if errors.Is(err, fs.ErrNotExist) {
		return mounts.Mount{}, fmt.Errorf("%w: mount %q", mounts.ErrNotFound, name)
	}
	if err != nil {
		return mounts.Mount{}, err
	}
	var r record
	if err := yaml.Unmarshal(b, &r); err != nil {
		return mounts.Mount{}, fmt.Errorf("%w: %s%s doesn't parse: %v", mounts.ErrInvalidArgument, name, ext, err)
	}
	if r.Name != name {
		return mounts.Mount{}, fmt.Errorf("%w: %s%s names mount %q", mounts.ErrInvalidArgument, name, ext, r.Name)
	}
	if err := mounts.ValidName(r.Name); err != nil {
		return mounts.Mount{}, err
	}
	return r.mount(), nil
}

// CreateMount writes m to a temp file, then hard-links the temp file to <name>.yaml. The link
// fails when the name exists, so of two processes creating one name, exactly one succeeds and
// the other gets ErrExists. A reader never sees a half-written record.
func (s *Store) CreateMount(ctx context.Context, m mounts.Mount) error {
	if err := mounts.ValidName(m.Name); err != nil {
		return err
	}
	b, err := yaml.Marshal(toRecord(m))
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	tmp := "." + m.Name + ".tmp-" + rand.Text()
	if err := root.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	defer root.Remove(tmp)
	if err := root.Link(tmp, m.Name+ext); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%w: mount %q", mounts.ErrExists, m.Name)
		}
		return err
	}
	return nil
}

// DeleteMount removes the record named name. A missing record is not an error.
func (s *Store) DeleteMount(ctx context.Context, name string) error {
	if err := mounts.ValidName(name); err != nil {
		return err
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := root.Remove(name + ext); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Generation hashes each record file's name, size and modification time, so a create or a delete
// by any process changes it. It reads every file's time because a folder's own modification time
// doesn't change on every create and delete on Windows.
func (s *Store) Generation(ctx context.Context) (string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ext) || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%d\x00%d\n", e.Name(), info.Size(), info.ModTime().UnixNano())
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
