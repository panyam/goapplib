package connections

import (
	"context"
	"errors"
	"fmt"

	"github.com/panyam/oneauth/keys"
	"github.com/panyam/oneauth/outbound"
)

// serverSecretPurpose names the key the service derives from the master key to seal server
// secrets. It differs from oneauth's user-token purpose, so a sealed server secret never opens as
// a person's token or the other way round.
const serverSecretPurpose = "goapplib-connection-secret"

// serverSecretSubject is the subject a server secret is kept under in the user-token store. The
// stores refuse an empty subject, and serverSecretProvider already names the connection.
const serverSecretSubject = "server"

// serverSecretProvider is the provider key a Sealed connection's secret is kept under in the
// user-token store. It can't collide with UserTokenProvider, so a person's connection with the
// same ID keeps its own record.
func serverSecretProvider(id string) string { return "goapplib-server-secret:" + id }

// SaveServerSecret seals secret as server connection id's secret: its token, App private key, AWS
// secret access key, GCP service account key or Entra client secret or certificate key, by Kind.
// The record must have Sealed set. Saving again replaces the secret, and every process sharing the
// stores picks up the new one on its next Credentials call. Saving clears a failed Status, and
// CanRead forgets its kept answers for requests with no viewer.
//
// It needs WithUserTokens for the store and the master key, and returns ErrNotConfigured without
// it. A missing connection is ErrNotFound. A person connection, a record without Sealed, or an
// empty secret is ErrInvalid. The service never hands the secret back.
func (s *Service) SaveServerSecret(ctx context.Context, id, secret string) error {
	if secret == "" {
		return fmt.Errorf("%w: %q: an empty secret", ErrInvalid, id)
	}
	c, err := s.store.GetConnection(ctx, "", id)
	if err != nil {
		return err
	}
	if !c.Sealed {
		return fmt.Errorf("%w: %q isn't sealed, so its secret is named by a reference", ErrInvalid, id)
	}
	if s.users == nil {
		return fmt.Errorf("%w: %q needs WithUserTokens to seal its secret", ErrNotConfigured, id)
	}
	s.mu.Lock()
	sl, err := s.sealerLocked()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	sealed, err := sl.Seal([]byte(secret), secretAAD(id))
	if err != nil {
		return fmt.Errorf("connections: %s: sealing: %w", id, err)
	}
	// Last writer wins: a conflict means another process saved in between, so read its version
	// and write over it.
	for range 5 {
		ver := int64(0)
		got, err := s.users.GetUserToken(ctx, &outbound.GetUserTokenRequest{Provider: serverSecretProvider(id), Subject: serverSecretSubject})
		switch {
		case err == nil:
			ver = got.Record.Version
		case !errors.Is(err, outbound.ErrUserTokenNotFound):
			return err
		}
		_, err = s.users.PutUserToken(ctx, &outbound.PutUserTokenRequest{Provider: serverSecretProvider(id), Subject: serverSecretSubject,
			Sealed: sealed, ExpectedVersion: ver})
		if errors.Is(err, outbound.ErrVersionConflict) {
			continue
		}
		if err != nil {
			return err
		}
		s.mu.Lock()
		delete(s.entries, connKey("", id))
		s.mu.Unlock()
		s.ForgetAccess("")
		if c.Status != StatusOK || c.LastError != "" {
			c.Status, c.LastError = StatusOK, ""
			return s.store.PutConnection(ctx, c)
		}
		return nil
	}
	return fmt.Errorf("connections: %s: saving the sealed secret kept conflicting: %w", id, outbound.ErrVersionConflict)
}

// secretAAD binds a sealed secret to its connection, so a record copied under another ID doesn't
// open.
func secretAAD(id string) []byte {
	return []byte(serverSecretProvider(id) + "\x00" + serverSecretSubject)
}

// sealerLocked returns the sealer for server secrets, making it on first use. Callers hold s.mu.
func (s *Service) sealerLocked() (*keys.Sealer, error) {
	if s.sealer != nil {
		return s.sealer, nil
	}
	key, err := s.masterKey()
	if err != nil {
		return nil, err
	}
	sl, err := keys.NewSealer(key, serverSecretPurpose)
	if err != nil {
		return nil, fmt.Errorf("connections: master key: %w", err)
	}
	s.sealer = sl
	return sl, nil
}

// sealedVersion returns the version of c's sealed secret, or ErrNoSecret when none is saved.
func (s *Service) sealedVersion(ctx context.Context, c Connection) (int64, error) {
	if s.users == nil {
		return 0, fmt.Errorf("%w: %q needs WithUserTokens for its sealed secret", ErrNotConfigured, c.ID)
	}
	got, err := s.users.GetUserToken(ctx, &outbound.GetUserTokenRequest{Provider: serverSecretProvider(c.ID), Subject: serverSecretSubject})
	if errors.Is(err, outbound.ErrUserTokenNotFound) {
		return 0, fmt.Errorf("%w: %q", ErrNoSecret, c.ID)
	}
	if err != nil {
		return 0, err
	}
	return got.Record.Version, nil
}

// openSealedLocked returns c's sealed secret and its version. Callers hold s.mu.
func (s *Service) openSealedLocked(ctx context.Context, c Connection) (string, int64, error) {
	if s.users == nil {
		return "", 0, fmt.Errorf("%w: %q needs WithUserTokens for its sealed secret", ErrNotConfigured, c.ID)
	}
	got, err := s.users.GetUserToken(ctx, &outbound.GetUserTokenRequest{Provider: serverSecretProvider(c.ID), Subject: serverSecretSubject})
	if errors.Is(err, outbound.ErrUserTokenNotFound) {
		return "", 0, fmt.Errorf("%w: %q", ErrNoSecret, c.ID)
	}
	if err != nil {
		return "", 0, err
	}
	sl, err := s.sealerLocked()
	if err != nil {
		return "", 0, err
	}
	plain, err := sl.Open(got.Record.Sealed, secretAAD(c.ID))
	if err != nil {
		return "", 0, fmt.Errorf("connections: %s: opening the sealed secret: %w", c.ID, err)
	}
	return string(plain), got.Record.Version, nil
}

// deleteSealed removes c's sealed secret. Deleting a missing one is not an error.
func (s *Service) deleteSealed(ctx context.Context, c Connection) error {
	if s.users == nil {
		return fmt.Errorf("%w: %q needs WithUserTokens to delete its sealed secret", ErrNotConfigured, c.ID)
	}
	_, err := s.users.DeleteUserToken(ctx, &outbound.DeleteUserTokenRequest{Provider: serverSecretProvider(c.ID), Subject: serverSecretSubject})
	return err
}

// secretFunc returns how c's secret fields resolve: the sealed secret for the field Sealed stands
// in for, and the reference otherwise. sealed is c's opened secret, or "" when c isn't Sealed.
func secretFunc(c Connection, sealed string) func(field, ref string) (string, error) {
	return func(field, ref string) (string, error) {
		if c.Sealed && (field == "key_ref" || field == "token_ref") {
			return sealed, nil
		}
		return resolve(c, field, ref)
	}
}
