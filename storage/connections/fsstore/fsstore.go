// Package fsstore keeps connection records in a folder, one YAML file per connection. A server
// connection is <id>.yaml at the top, and a person's is people/<owner>/<id>.yaml, with the owner's
// subject base64url-encoded so no subject can name a path. It suits a single server or several
// processes sharing one disk, and a test. The records hold no secrets
// (connections.Connection.Validate refuses them), so the folder can be backed up as it is.
//
// Before goapplib 0.7.23 a person's connection was <id>.yaml at the top too. Such a record still
// reads back as its owner's, and the next write to that ID moves it under people/.
package fsstore

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
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
	Pasted         bool      `yaml:"pasted,omitempty"`
	Status         string    `yaml:"status,omitempty"`
	LastError      string    `yaml:"last_error,omitempty"`
	Created        time.Time `yaml:"created"`
	LastUsed       time.Time `yaml:"last_used,omitempty"`
	AccessKeyID    string    `yaml:"access_key_id,omitempty"`
	RoleARN        string    `yaml:"role_arn,omitempty"`
	Region         string    `yaml:"region,omitempty"`
	TenantID       string    `yaml:"tenant_id,omitempty"`
	ClientID       string    `yaml:"client_id,omitempty"`
	EntraAuth      string    `yaml:"entra_auth,omitempty"`
	CertRef        string    `yaml:"cert_ref,omitempty"`
	Sealed         bool      `yaml:"sealed,omitempty"`
}

func toRecord(c connections.Connection) record {
	return record{c.ID, string(c.Kind), string(c.Scope), c.Owner, c.Name, c.BaseURL, c.AppID,
		c.InstallationID, c.KeyRef, c.TokenRef, c.Pasted, string(c.Status), c.LastError, c.Created.UTC(), c.LastUsed.UTC(), c.AccessKeyID, c.RoleARN, c.Region,
		c.TenantID, c.ClientID, string(c.EntraAuth), c.CertRef, c.Sealed}
}

func (r record) connection() connections.Connection {
	return connections.Connection{ID: r.ID, Kind: connections.Kind(r.Kind), Scope: connections.Scope(r.Scope),
		Owner: r.Owner, Name: r.Name, BaseURL: r.BaseURL, AppID: r.AppID, InstallationID: r.InstallationID,
		KeyRef: r.KeyRef, TokenRef: r.TokenRef, Pasted: r.Pasted, Status: connections.Status(r.Status), LastError: r.LastError,
		Created: r.Created, LastUsed: r.LastUsed, AccessKeyID: r.AccessKeyID, RoleARN: r.RoleARN, Region: r.Region,
		TenantID: r.TenantID, ClientID: r.ClientID, EntraAuth: connections.EntraAuth(r.EntraAuth), CertRef: r.CertRef, Sealed: r.Sealed}
}

// peopleDir holds a folder per person, each with that person's connections.
const peopleDir = "people"

// file is the path of owner's record id, relative to the store's folder.
func file(owner, id string) string {
	if owner == "" {
		return id + ".yaml"
	}
	return path.Join(personDir(owner), id+".yaml")
}

func personDir(owner string) string {
	return path.Join(peopleDir, base64.RawURLEncoding.EncodeToString([]byte(owner)))
}

// GetConnection reads owner's record id. A missing record, another owner's, or an id that isn't a
// valid connection ID returns connections.ErrNotFound. A file whose record names another ID or
// owner, or that no longer passes Validate, returns an error wrapping connections.ErrInvalid.
func (s *Store) GetConnection(_ context.Context, owner, id string) (connections.Connection, error) {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return connections.Connection{}, fmt.Errorf("fsstore: %w", err)
	}
	defer root.Close()
	if !validID(id) {
		return connections.Connection{}, fmt.Errorf("%w: %q", connections.ErrNotFound, id)
	}
	if owner != "" {
		c, err := s.read(root, file(owner, id), id)
		if err == nil && c.Owner != owner {
			return connections.Connection{}, fmt.Errorf("%w: %s holds another owner's connection", connections.ErrInvalid, file(owner, id))
		}
		if !errors.Is(err, connections.ErrNotFound) {
			return c, err
		}
	}
	// The top level holds server records, and person records saved before people/ existed.
	c, err := s.read(root, file("", id), id)
	if err == nil && c.Owner != owner {
		return connections.Connection{}, fmt.Errorf("%w: %q", connections.ErrNotFound, id)
	}
	return c, err
}

// read reads the record at name, which has to hold connection id and pass Validate.
func (s *Store) read(root *os.Root, name, id string) (connections.Connection, error) {
	b, err := root.ReadFile(name)
	if errors.Is(err, fs.ErrNotExist) {
		return connections.Connection{}, fmt.Errorf("%w: %q", connections.ErrNotFound, id)
	}
	if err != nil {
		return connections.Connection{}, fmt.Errorf("fsstore: %w", err)
	}
	var r record
	if err := yaml.Unmarshal(b, &r); err != nil {
		return connections.Connection{}, fmt.Errorf("%w: %s: %v", connections.ErrInvalid, name, err)
	}
	if r.ID != id {
		return connections.Connection{}, fmt.Errorf("%w: %s holds connection %q", connections.ErrInvalid, name, r.ID)
	}
	c := r.connection()
	if err := c.Validate(); err != nil {
		return connections.Connection{}, fmt.Errorf("%s: %w", name, err)
	}
	return c, nil
}

// validID reports whether id can name a record file. Connection IDs follow the mount name rule,
// which keeps "../x" and "a/b" from naming a path, so a lookup of one is a plain not-found.
func validID(id string) bool {
	return mounts.ValidName(id) == nil
}

// ListConnections returns owner's records in ID order. An empty owner lists the server-scoped
// ones. A file that doesn't read back as a valid record under its own name fails the whole list,
// so a damaged record is noticed rather than skipped.
func (s *Store) ListConnections(_ context.Context, owner string) ([]connections.Connection, error) {
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return nil, fmt.Errorf("fsstore: %w", err)
	}
	defer root.Close()
	byID := map[string]connections.Connection{}
	// The top level holds server records, and person records from before people/.
	top, err := s.readDir(root, ".")
	if err != nil {
		return nil, err
	}
	for _, c := range top {
		if c.Owner == owner {
			byID[c.ID] = c
		}
	}
	if owner != "" {
		mine, err := s.readDir(root, personDir(owner))
		if err != nil {
			return nil, err
		}
		for _, c := range mine {
			byID[c.ID] = c
		}
	}
	out := make([]connections.Connection, 0, len(byID))
	for _, c := range byID {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// readDir reads every record in dir. A missing dir has none.
func (s *Store) readDir(root *os.Root, dir string) ([]connections.Connection, error) {
	entries, err := fs.ReadDir(root.FS(), dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("fsstore: %w", err)
	}
	var out []connections.Connection
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".yaml")
		if !ok || !e.Type().IsRegular() {
			continue
		}
		c, err := s.read(root, path.Join(dir, e.Name()), id)
		if err != nil {
			return nil, err
		}
		if dir != "." && path.Dir(file(c.Owner, c.ID)) != dir {
			return nil, fmt.Errorf("%w: %s holds another owner's connection", connections.ErrInvalid, path.Join(dir, e.Name()))
		}
		out = append(out, c)
	}
	return out, nil
}

// PutConnection validates c, then writes it through a temp file and a rename. An invalid record
// returns connections.ErrInvalid and writes nothing. A person's record from before people/ under
// the same ID moves there first, so a server record can't overwrite it.
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
	if err := s.moveOld(root, c.ID); err != nil {
		return err
	}
	return write(root, file(c.Owner, c.ID), b)
}

// write writes b to name through a temp file beside it and a rename.
func write(root *os.Root, name string, b []byte) error {
	dir := path.Dir(name)
	if err := root.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("fsstore: %w", err)
	}
	tmp := path.Join(dir, fmt.Sprintf(".%s.%d.tmp", path.Base(name), time.Now().UnixNano()))
	if err := root.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("fsstore: %w", err)
	}
	if err := root.Rename(tmp, name); err != nil {
		_ = root.Remove(tmp)
		return fmt.Errorf("fsstore: %w", err)
	}
	return nil
}

// moveOld moves a person's record saved as <id>.yaml at the top, from before people/, to its
// place under people/. When that place already has a record, the newer one there wins and the old
// file goes. Callers hold s.mu.
func (s *Store) moveOld(root *os.Root, id string) error {
	b, err := root.ReadFile(file("", id))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("fsstore: %w", err)
	}
	var r record
	if err := yaml.Unmarshal(b, &r); err != nil || r.Owner == "" || r.ID != id {
		return nil
	}
	dest := file(r.Owner, id)
	if _, err := root.Stat(dest); errors.Is(err, fs.ErrNotExist) {
		if err := write(root, dest, b); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("fsstore: %w", err)
	}
	if err := root.Remove(file("", id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("fsstore: %w", err)
	}
	return nil
}

// DeleteConnection removes owner's record id, including one saved before people/. A missing
// record is not an error.
func (s *Store) DeleteConnection(_ context.Context, owner, id string) error {
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
	if err := s.moveOld(root, id); err != nil {
		return err
	}
	if err := root.Remove(file(owner, id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("fsstore: %w", err)
	}
	return nil
}
