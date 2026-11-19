// Package fsstore keeps connection records in a folder, one YAML file per connection named
// <id>.yaml. It suits a single server or several processes sharing one disk, and a test. The
// records hold no secrets (connections.Connection.Validate refuses them), so the folder can be
// backed up as it is.
package fsstore

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/panyam/goapplib/storage/connections"
	"github.com/panyam/goapplib/storage/mounts"
	"gopkg.in/yaml.v3"
)

// Store is a connections.Store over one folder. Writes go through a temp file and a rename, so a
// reader in another process sees a whole record or the one before it. Two processes writing the
// same record at once leave whichever renamed last.
type Store struct {
	dir string
	mu  sync.Mutex
}

var _ connections.Store = (*Store)(nil)

// New returns a store over dir, creating the folder if it is missing.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("fsstore: %w", err)
	}
	return &Store{dir: dir}, nil
}

// record is the YAML form of a connection. Its tags are the file format, so renaming a Go field
// doesn't change what is on disk.
type record struct {
	ID             string    `yaml:"id"`
	Kind           string    `yaml:"kind"`
	Scope          string    `yaml:"scope"`
	Owner          string    `yaml:"owner,omitempty"`
	Name           string    `yaml:"name,omitempty"`
	BaseURL        string    `yaml:"base_url,omitempty"`
	AppID          string    `yaml:"app_id,omitempty"`
	InstallationID int64     `yaml:"installation_id,omitempty"`
	KeyRef         string    `yaml:"key_ref,omitempty"`
	TokenRef       string    `yaml:"token_ref,omitempty"`
	Status         string    `yaml:"status,omitempty"`
	LastError      string    `yaml:"last_error,omitempty"`
	Created        time.Time `yaml:"created"`
	LastUsed       time.Time `yaml:"last_used,omitempty"`
}

func toRecord(c connections.Connection) record {
	return record{c.ID, string(c.Kind), string(c.Scope), c.Owner, c.Name, c.BaseURL, c.AppID,
		c.InstallationID, c.KeyRef, c.TokenRef, string(c.Status), c.LastError, c.Created.UTC(), c.LastUsed.UTC()}
}

func (r record) connection() connections.Connection {
	return connections.Connection{ID: r.ID, Kind: connections.Kind(r.Kind), Scope: connections.Scope(r.Scope),
		Owner: r.Owner, Name: r.Name, BaseURL: r.BaseURL, AppID: r.AppID, InstallationID: r.InstallationID,
		KeyRef: r.KeyRef, TokenRef: r.TokenRef, Status: connections.Status(r.Status), LastError: r.LastError,
		Created: r.Created, LastUsed: r.LastUsed}
}

// GetConnection reads <id>.yaml. A missing file, or an id that isn't a valid connection ID,
// returns connections.ErrNotFound. A file whose record names another ID, or that no longer
// passes Validate, returns an error wrapping connections.ErrInvalid.
func (s *Store) GetConnection(_ context.Context, id string) (connections.Connection, error) {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return connections.Connection{}, fmt.Errorf("fsstore: %w", err)
	}
	defer root.Close()
	return s.read(root, id)
}

func (s *Store) read(root *os.Root, id string) (connections.Connection, error) {
	if !validID(id) {
		return connections.Connection{}, fmt.Errorf("%w: %q", connections.ErrNotFound, id)
	}
	b, err := root.ReadFile(id + ".yaml")
	if errors.Is(err, fs.ErrNotExist) {
		return connections.Connection{}, fmt.Errorf("%w: %q", connections.ErrNotFound, id)
	}
	if err != nil {
		return connections.Connection{}, fmt.Errorf("fsstore: %w", err)
	}
	var r record
	if err := yaml.Unmarshal(b, &r); err != nil {
		return connections.Connection{}, fmt.Errorf("%w: %s.yaml: %v", connections.ErrInvalid, id, err)
	}
	if r.ID != id {
		return connections.Connection{}, fmt.Errorf("%w: %s.yaml holds connection %q", connections.ErrInvalid, id, r.ID)
	}
	c := r.connection()
	if err := c.Validate(); err != nil {
		return connections.Connection{}, fmt.Errorf("%s.yaml: %w", id, err)
	}
	return c, nil
}

// validID reports whether id can name a record file. Connection IDs follow the mount name rule,
// which keeps "../x" and "a/b" from naming a path, so a lookup of one is a plain not-found.
func validID(id string) bool {
	return mounts.ValidName(id) == nil
}

// ListConnections reads every record and returns those owned by owner, in ID order. An empty
// owner lists the server-scoped ones. A file that doesn't read back as a valid record under its
// own name fails the whole list, so a damaged record is noticed rather than skipped.
func (s *Store) ListConnections(_ context.Context, owner string) ([]connections.Connection, error) {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, fmt.Errorf("fsstore: %w", err)
	}
	defer root.Close()
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, fmt.Errorf("fsstore: %w", err)
	}
	var out []connections.Connection
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".yaml")
		if !ok || !e.Type().IsRegular() {
			continue
		}
		c, err := s.read(root, id)
		if err != nil {
			return nil, err
		}
		if c.Owner == owner {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// PutConnection validates c, then writes <c.ID>.yaml through a temp file and a rename. An invalid
// record returns connections.ErrInvalid and writes nothing.
func (s *Store) PutConnection(_ context.Context, c connections.Connection) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := yaml.Marshal(toRecord(c))
	if err != nil {
		return fmt.Errorf("fsstore: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return fmt.Errorf("fsstore: %w", err)
	}
	defer root.Close()
	tmp := fmt.Sprintf(".%s.%d.tmp", c.ID, time.Now().UnixNano())
	if err := root.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("fsstore: %w", err)
	}
	if err := root.Rename(tmp, c.ID+".yaml"); err != nil {
		_ = root.Remove(tmp)
		return fmt.Errorf("fsstore: %w", err)
	}
	return nil
}

// DeleteConnection removes <id>.yaml. A missing record is not an error.
func (s *Store) DeleteConnection(_ context.Context, id string) error {
	if !validID(id) {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return fmt.Errorf("fsstore: %w", err)
	}
	defer root.Close()
	if err := root.Remove(id + ".yaml"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("fsstore: %w", err)
	}
	return nil
}
