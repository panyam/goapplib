//go:build !wasm
// +build !wasm

// Package gae provides a Google Cloud Datastore session store for scs (alexedwards/scs/v2).
// It stores session data in Datastore with automatic expiration cleanup.
//
// # Usage
//
//	client, _ := datastore.NewClient(ctx, projectID)
//	sessionStore := gae.NewGCDSessionStore(client, "", time.Hour) // cleanup every hour
//	session := scs.New()
//	session.Store = sessionStore
//
// # Datastore Kind
//
// Sessions are stored with the kind "AuthSession" and the following properties:
//   - Token: string (used as key name)
//   - ExpiresAt: time.Time (indexed for cleanup queries)
//   - Data: []byte (session data, noindex)
//
// # Namespacing
//
// The store supports Datastore namespaces for multi-tenant applications:
//
//	sessionStore := gae.NewGCDSessionStore(client, "tenant-123", time.Hour)
package gae

import (
	"context"
	"log"
	"time"

	"cloud.google.com/go/datastore"
)

const sessionKind = "AuthSession"

// AuthSession represents a session entity in Datastore.
type AuthSession struct {
	Token     string    `datastore:"-"` // Key name, not stored as property
	ExpiresAt time.Time `datastore:"expiresAt"`
	Data      []byte    `datastore:"data,noindex"`
}

// GCDSessionStore implements the scs.Store interface using Google Cloud Datastore.
type GCDSessionStore struct {
	client      *datastore.Client
	namespace   string
	stopCleanup chan bool
}

// NewGCDSessionStore creates a new Datastore-backed session store.
// The cleanupInterval parameter controls how frequently expired session data is removed.
// Setting it to 0 disables the background cleanup goroutine.
func NewGCDSessionStore(client *datastore.Client, namespace string, cleanupInterval time.Duration) *GCDSessionStore {
	store := &GCDSessionStore{
		client:    client,
		namespace: namespace,
	}

	if cleanupInterval > 0 {
		go store.startCleanup(cleanupInterval)
	}

	return store
}

// namespacedKey creates a Datastore key with the configured namespace.
func (s *GCDSessionStore) namespacedKey(token string) *datastore.Key {
	key := datastore.NameKey(sessionKind, token, nil)
	key.Namespace = s.namespace
	return key
}

// namespacedQuery creates a Datastore query with the configured namespace.
func (s *GCDSessionStore) namespacedQuery() *datastore.Query {
	query := datastore.NewQuery(sessionKind)
	if s.namespace != "" {
		query = query.Namespace(s.namespace)
	}
	return query
}

// FindCtx returns the data for a given session token.
// If the session token is not found or is expired, the returned exists flag will be false.
func (s *GCDSessionStore) FindCtx(ctx context.Context, token string) ([]byte, bool, error) {
	key := s.namespacedKey(token)
	var session AuthSession
	err := s.client.Get(ctx, key, &session)
	if err != nil {
		if err == datastore.ErrNoSuchEntity {
			return nil, false, nil
		}
		return nil, false, err
	}

	if time.Now().After(session.ExpiresAt) {
		return nil, false, nil
	}

	return session.Data, true, nil
}

// CommitCtx adds a session token and data to Datastore with the given expiry time.
// If the session token already exists, the data and expiry time are updated.
func (s *GCDSessionStore) CommitCtx(ctx context.Context, token string, data []byte, expiry time.Time) error {
	key := s.namespacedKey(token)
	session := &AuthSession{
		Token:     token,
		ExpiresAt: expiry,
		Data:      data,
	}
	_, err := s.client.Put(ctx, key, session)
	return err
}

// DeleteCtx removes a session token and corresponding data from Datastore.
func (s *GCDSessionStore) DeleteCtx(ctx context.Context, token string) error {
	key := s.namespacedKey(token)
	return s.client.Delete(ctx, key)
}

// AllCtx returns a map containing the token and data for all active (non-expired) sessions.
func (s *GCDSessionStore) AllCtx(ctx context.Context) (map[string][]byte, error) {
	query := s.namespacedQuery().FilterField("expiresAt", ">", time.Now())

	var sessions []AuthSession
	keys, err := s.client.GetAll(ctx, query, &sessions)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]byte)
	for i, key := range keys {
		result[key.Name] = sessions[i].Data
	}
	return result, nil
}

// startCleanup runs a background goroutine that periodically deletes expired sessions.
func (s *GCDSessionStore) startCleanup(interval time.Duration) {
	s.stopCleanup = make(chan bool)
	ticker := time.NewTicker(interval)
	for {
		select {
		case <-ticker.C:
			if err := s.deleteExpired(context.Background()); err != nil {
				log.Printf("GCDSessionStore: cleanup error: %v", err)
			}
		case <-s.stopCleanup:
			ticker.Stop()
			return
		}
	}
}

// StopCleanup terminates the background cleanup goroutine.
func (s *GCDSessionStore) StopCleanup() {
	if s.stopCleanup != nil {
		s.stopCleanup <- true
	}
}

// deleteExpired removes all expired sessions from Datastore.
func (s *GCDSessionStore) deleteExpired(ctx context.Context) error {
	query := s.namespacedQuery().FilterField("expiresAt", "<", time.Now()).KeysOnly()

	keys, err := s.client.GetAll(ctx, query, nil)
	if err != nil {
		return err
	}

	if len(keys) == 0 {
		return nil
	}

	// Delete in batches of 500 (Datastore limit)
	batchSize := 500
	for i := 0; i < len(keys); i += batchSize {
		end := i + batchSize
		if end > len(keys) {
			end = len(keys)
		}
		if err := s.client.DeleteMulti(ctx, keys[i:end]); err != nil {
			return err
		}
	}

	log.Printf("GCDSessionStore: cleaned up %d expired sessions", len(keys))
	return nil
}

// Non-context versions required by scs.Store interface.
// These panic because scs should always use the Ctx versions.

func (s *GCDSessionStore) Find(token string) ([]byte, bool, error) {
	panic("GCDSessionStore: use FindCtx instead")
}

func (s *GCDSessionStore) Commit(token string, data []byte, expiry time.Time) error {
	panic("GCDSessionStore: use CommitCtx instead")
}

func (s *GCDSessionStore) Delete(token string) error {
	panic("GCDSessionStore: use DeleteCtx instead")
}
