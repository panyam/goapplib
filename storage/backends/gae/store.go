// Package gaestore keeps mount records and connection records in Google Cloud Datastore (or
// Firestore in Datastore mode), so several servers can share them without a shared folder. A
// namespace keeps one app's or one tenant's records apart from another's.
//
// It is a Go module of its own, github.com/panyam/goapplib/storage/backends/gae, so an app that
// keeps its records in a folder never downloads the Datastore SDK.
package gaestore

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/panyam/goapplib/storage/connections"
	"github.com/panyam/goapplib/storage/mounts"
)

// The entity kinds. They are the schema, so renaming a Go type doesn't change them.
const (
	mountKind      = "GoapplibMount"
	generationKind = "GoapplibMountGeneration"
	connectionKind = "GoapplibConnection"
	// ownerKind is the parent key of a person's connections. It has no entities of its own.
	ownerKind = "GoapplibConnectionOwner"
)

// maxAttempts is how often a contended transaction is tried before it fails. Every create and
// delete writes the one generation entity, so concurrent writers contend on it. The limit is
// above Datastore's default of 3 to leave room for a burst of them.
const maxAttempts = 32

// mountEntity is a mount record. Its key's name is the mount's name, so the key keeps names unique
// and orders the list.
type mountEntity struct {
	Kind       string    `datastore:"kind,noindex"`
	Source     string    `datastore:"source,noindex"`
	Ref        string    `datastore:"ref,noindex"`
	Version    string    `datastore:"version,noindex"`
	Mode       string    `datastore:"mode,noindex"`
	Connection string    `datastore:"connection,noindex"`
	ConnOwner  string    `datastore:"connection_owner,noindex"`
	Manifest   string    `datastore:"manifest,noindex"`
	Created    time.Time `datastore:"created,noindex"`
}

type generationEntity struct {
	Counter int64 `datastore:"counter,noindex"`
}

func toMountEntity(m mounts.Mount) *mountEntity {
	return &mountEntity{string(m.Kind), m.Source, m.Ref, m.Version, string(m.Mode), m.Connection, m.ConnectionOwner, m.Manifest, m.Created.UTC()}
}

func (e *mountEntity) mount(name string) mounts.Mount {
	return mounts.Mount{Name: name, Kind: mounts.Kind(e.Kind), Source: e.Source, Ref: e.Ref, Version: e.Version,
		Mode: mounts.Mode(e.Mode), Connection: e.Connection, ConnectionOwner: e.ConnOwner, Manifest: e.Manifest, Created: e.Created.UTC()}
}

// MountStore is a mounts.RecordStore over Datastore. Several servers can share one namespace: a
// create is exclusive across them, and each sees the others' changes through Generation.
type MountStore struct {
	client    *datastore.Client
	namespace string
}

// NewMountStore returns a store over the mount records in namespace. An empty namespace is
// Datastore's default one.
func NewMountStore(client *datastore.Client, namespace string) *MountStore {
	return &MountStore{client: client, namespace: namespace}
}

func (s *MountStore) key(name string) *datastore.Key {
	k := datastore.NameKey(mountKind, name, nil)
	k.Namespace = s.namespace
	return k
}

func (s *MountStore) generationKey() *datastore.Key {
	k := datastore.NameKey(generationKind, "generation", nil)
	k.Namespace = s.namespace
	return k
}

// ListMounts returns every record, in name order.
func (s *MountStore) ListMounts(ctx context.Context) ([]mounts.Mount, error) {
	q := datastore.NewQuery(mountKind).Namespace(s.namespace).Order("__key__")
	var es []*mountEntity
	keys, err := s.client.GetAll(ctx, q, &es)
	if err != nil {
		return nil, err
	}
	out := make([]mounts.Mount, len(es))
	for i, e := range es {
		out[i] = e.mount(keys[i].Name)
	}
	return out, nil
}

// GetMount returns the record named name, or mounts.ErrNotFound.
func (s *MountStore) GetMount(ctx context.Context, name string) (mounts.Mount, error) {
	if mounts.ValidName(name) != nil {
		return mounts.Mount{}, fmt.Errorf("%w: mount %q", mounts.ErrNotFound, name)
	}
	var e mountEntity
	err := s.client.Get(ctx, s.key(name), &e)
	if errors.Is(err, datastore.ErrNoSuchEntity) {
		return mounts.Mount{}, fmt.Errorf("%w: mount %q", mounts.ErrNotFound, name)
	}
	if err != nil {
		return mounts.Mount{}, err
	}
	return e.mount(name), nil
}

// CreateMount writes m and bumps the generation counter in one transaction. A record with m.Name
// returns mounts.ErrExists, so of two servers creating the same name, one wins. A bad name returns
// mounts.ErrInvalidArgument.
func (s *MountStore) CreateMount(ctx context.Context, m mounts.Mount) error {
	if err := mounts.ValidName(m.Name); err != nil {
		return err
	}
	_, err := s.client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var existing mountEntity
		err := tx.Get(s.key(m.Name), &existing)
		if err == nil {
			return fmt.Errorf("%w: mount %q", mounts.ErrExists, m.Name)
		}
		if !errors.Is(err, datastore.ErrNoSuchEntity) {
			return err
		}
		if _, err := tx.Put(s.key(m.Name), toMountEntity(m)); err != nil {
			return err
		}
		return s.bump(tx)
	}, datastore.MaxAttempts(maxAttempts))
	return err
}

// DeleteMount removes the record named name, and bumps the generation counter in the same
// transaction when there was one. Deleting a missing record is not an error.
func (s *MountStore) DeleteMount(ctx context.Context, name string) error {
	if mounts.ValidName(name) != nil {
		return nil
	}
	_, err := s.client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		var existing mountEntity
		err := tx.Get(s.key(name), &existing)
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := tx.Delete(s.key(name)); err != nil {
			return err
		}
		return s.bump(tx)
	}, datastore.MaxAttempts(maxAttempts))
	return err
}

func (s *MountStore) bump(tx *datastore.Transaction) error {
	var g generationEntity
	if err := tx.Get(s.generationKey(), &g); err != nil && !errors.Is(err, datastore.ErrNoSuchEntity) {
		return err
	}
	g.Counter++
	_, err := tx.Put(s.generationKey(), &g)
	return err
}

// Generation returns the counter that every create and delete bumps, as a decimal string. It is
// "0" before the first write.
func (s *MountStore) Generation(ctx context.Context) (string, error) {
	var g generationEntity
	err := s.client.Get(ctx, s.generationKey(), &g)
	if err != nil && !errors.Is(err, datastore.ErrNoSuchEntity) {
		return "", err
	}
	return strconv.FormatInt(g.Counter, 10), nil
}

// connectionEntity is a connection record. Its key's name is the connection's ID. A server
// connection's key has no parent, and a person's has an ownerKind parent named for the owner.
// Before goapplib 0.7.23 a person's connection had no parent either; such an entity still reads
// back as its owner's, and the next write to that ID moves it under the parent. It holds secret
// references, never secrets, since PutConnection validates first. Only Owner is indexed, for
// ListConnections.
type connectionEntity struct {
	Kind           string    `datastore:"kind,noindex"`
	Scope          string    `datastore:"scope,noindex"`
	Owner          string    `datastore:"owner"`
	Name           string    `datastore:"name,noindex"`
	BaseURL        string    `datastore:"base_url,noindex"`
	AppID          string    `datastore:"app_id,noindex"`
	InstallationID int64     `datastore:"installation_id,noindex"`
	KeyRef         string    `datastore:"key_ref,noindex"`
	TokenRef       string    `datastore:"token_ref,noindex"`
	Pasted         bool      `datastore:"pasted,noindex"`
	Status         string    `datastore:"status,noindex"`
	LastError      string    `datastore:"last_error,noindex"`
	Created        time.Time `datastore:"created,noindex"`
	LastUsed       time.Time `datastore:"last_used,noindex"`
	AccessKeyID    string    `datastore:"access_key_id,noindex"`
	RoleARN        string    `datastore:"role_arn,noindex"`
	Region         string    `datastore:"region,noindex"`
	TenantID       string    `datastore:"tenant_id,noindex"`
	ClientID       string    `datastore:"client_id,noindex"`
	EntraAuth      string    `datastore:"entra_auth,noindex"`
	CertRef        string    `datastore:"cert_ref,noindex"`
	Sealed         bool      `datastore:"sealed,noindex"`
}

func toConnectionEntity(c connections.Connection) *connectionEntity {
	return &connectionEntity{string(c.Kind), string(c.Scope), c.Owner, c.Name, c.BaseURL, c.AppID,
		c.InstallationID, c.KeyRef, c.TokenRef, c.Pasted, string(c.Status), c.LastError, c.Created.UTC(), c.LastUsed.UTC(), c.AccessKeyID, c.RoleARN, c.Region,
		c.TenantID, c.ClientID, string(c.EntraAuth), c.CertRef, c.Sealed}
}

func (e *connectionEntity) connection(id string) connections.Connection {
	return connections.Connection{ID: id, Kind: connections.Kind(e.Kind), Scope: connections.Scope(e.Scope),
		Owner: e.Owner, Name: e.Name, BaseURL: e.BaseURL, AppID: e.AppID, InstallationID: e.InstallationID,
		KeyRef: e.KeyRef, TokenRef: e.TokenRef, Pasted: e.Pasted, Status: connections.Status(e.Status), LastError: e.LastError,
		Created: e.Created.UTC(), LastUsed: e.LastUsed.UTC(), AccessKeyID: e.AccessKeyID, RoleARN: e.RoleARN, Region: e.Region,
		TenantID: e.TenantID, ClientID: e.ClientID, EntraAuth: connections.EntraAuth(e.EntraAuth), CertRef: e.CertRef, Sealed: e.Sealed}
}

// ConnectionStore is a connections.Store over Datastore.
type ConnectionStore struct {
	client    *datastore.Client
	namespace string
}

// NewConnectionStore returns a store over the connection records in namespace. An empty namespace
// is Datastore's default one.
func NewConnectionStore(client *datastore.Client, namespace string) *ConnectionStore {
	return &ConnectionStore{client: client, namespace: namespace}
}

// key is owner's connection id. The owner's subject is base64url-encoded in the parent's name,
// since Datastore refuses some names, such as ones of the form __x__.
func (s *ConnectionStore) key(owner, id string) *datastore.Key {
	var parent *datastore.Key
	if owner != "" {
		parent = datastore.NameKey(ownerKind, base64.RawURLEncoding.EncodeToString([]byte(owner)), nil)
		parent.Namespace = s.namespace
	}
	k := datastore.NameKey(connectionKind, id, parent)
	k.Namespace = s.namespace
	return k
}

// GetConnection returns owner's record with that ID, or connections.ErrNotFound. With no record
// under the owner's parent, it falls back to an entity from before parents existed.
func (s *ConnectionStore) GetConnection(ctx context.Context, owner, id string) (connections.Connection, error) {
	if id == "" {
		return connections.Connection{}, fmt.Errorf("%w: empty connection ID", connections.ErrNotFound)
	}
	notFound := fmt.Errorf("%w: connection %q", connections.ErrNotFound, id)
	var e connectionEntity
	if owner != "" {
		err := s.client.Get(ctx, s.key(owner, id), &e)
		if err == nil {
			return e.connection(id), nil
		}
		if !errors.Is(err, datastore.ErrNoSuchEntity) {
			return connections.Connection{}, err
		}
	}
	err := s.client.Get(ctx, s.key("", id), &e)
	if errors.Is(err, datastore.ErrNoSuchEntity) {
		return connections.Connection{}, notFound
	}
	if err != nil {
		return connections.Connection{}, err
	}
	if e.Owner != owner {
		return connections.Connection{}, notFound
	}
	return e.connection(id), nil
}

// ListConnections returns the connections owned by owner, in ID order. An empty owner lists the
// server-scoped ones.
func (s *ConnectionStore) ListConnections(ctx context.Context, owner string) ([]connections.Connection, error) {
	q := datastore.NewQuery(connectionKind).Namespace(s.namespace).FilterField("owner", "=", owner)
	var es []*connectionEntity
	keys, err := s.client.GetAll(ctx, q, &es)
	if err != nil {
		return nil, err
	}
	// An owner can have an old parentless entity and a new one under their parent for one ID while
	// a move is in flight. The one under the parent is newer.
	byID := map[string]connections.Connection{}
	for i, e := range es {
		if _, seen := byID[keys[i].Name]; seen && keys[i].Parent == nil {
			continue
		}
		byID[keys[i].Name] = e.connection(keys[i].Name)
	}
	out := make([]connections.Connection, 0, len(byID))
	for _, c := range byID {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// PutConnection validates c, then writes it, replacing owner's record with c.ID. An invalid record
// returns connections.ErrInvalid and writes nothing. A person's entity from before parents existed
// with the same ID moves under its owner's parent in the same transaction, so a server record can't
// overwrite it.
func (s *ConnectionStore) PutConnection(ctx context.Context, c connections.Connection) error {
	if err := c.Validate(); err != nil {
		return err
	}
	_, err := s.client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		if err := s.moveOld(tx, c.ID); err != nil {
			return err
		}
		_, err := tx.Put(s.key(c.Owner, c.ID), toConnectionEntity(c))
		return err
	})
	return err
}

// moveOld moves a person's parentless entity id, from before parents existed, under its owner's
// parent. When the owner already has a record there, that newer one wins and the old one goes.
func (s *ConnectionStore) moveOld(tx *datastore.Transaction, id string) error {
	var e connectionEntity
	err := tx.Get(s.key("", id), &e)
	if errors.Is(err, datastore.ErrNoSuchEntity) {
		return nil
	}
	if err != nil {
		return err
	}
	if e.Owner == "" {
		return nil
	}
	var mine connectionEntity
	err = tx.Get(s.key(e.Owner, id), &mine)
	if errors.Is(err, datastore.ErrNoSuchEntity) {
		if _, err := tx.Put(s.key(e.Owner, id), &e); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return tx.Delete(s.key("", id))
}

// DeleteConnection removes owner's record with that ID, including one from before parents existed.
// Deleting a missing record is not an error.
func (s *ConnectionStore) DeleteConnection(ctx context.Context, owner, id string) error {
	if id == "" {
		return nil
	}
	_, err := s.client.RunInTransaction(ctx, func(tx *datastore.Transaction) error {
		if err := s.moveOld(tx, id); err != nil {
			return err
		}
		return tx.Delete(s.key(owner, id))
	})
	return err
}

var (
	_ mounts.RecordStore = (*MountStore)(nil)
	_ connections.Store  = (*ConnectionStore)(nil)
)
