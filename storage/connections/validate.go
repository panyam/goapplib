package connections

import (
	"fmt"
	"strings"

	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/oneauth/outbound"
)

// Validate checks that c's fields fit its Kind and Scope, and that every secret field is a secret
// reference rather than a secret. It never resolves a reference, so it works without the secret
// present, and its errors never repeat a field's value. Every failure wraps ErrInvalid.
func (c Connection) Validate() error {
	if err := mounts.ValidName(c.ID); err != nil {
		return fmt.Errorf("%w: id: %v", ErrInvalid, err)
	}
	switch c.Scope {
	case ScopeServer:
		if c.Owner != "" {
			return fmt.Errorf("%w: a server connection has no owner", ErrInvalid)
		}
	case ScopePerson:
		if c.Owner == "" {
			return fmt.Errorf("%w: a person connection needs an owner", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: unknown scope %q", ErrInvalid, c.Scope)
	}

	if c.Pasted && c.Kind != KindGitHubUser {
		return fmt.Errorf("%w: only a GitHub user connection has a pasted token", ErrInvalid)
	}
	switch c.Kind {
	case KindGitHubApp:
		if c.Scope != ScopeServer {
			return fmt.Errorf("%w: a GitHub App installation signs in as the server", ErrInvalid)
		}
		if c.AppID == "" || c.InstallationID <= 0 {
			return fmt.Errorf("%w: a GitHub App connection needs an app ID and an installation ID", ErrInvalid)
		}
		if err := secretRef("key_ref", c.KeyRef); err != nil {
			return err
		}
		if c.TokenRef != "" {
			return fmt.Errorf("%w: a GitHub App connection has no token_ref", ErrInvalid)
		}
	case KindGitHubToken:
		if err := secretRef("token_ref", c.TokenRef); err != nil {
			return err
		}
		if c.KeyRef != "" || c.AppID != "" || c.InstallationID != 0 {
			return fmt.Errorf("%w: a token connection has no app fields", ErrInvalid)
		}
	case KindGitHubUser:
		if c.Scope != ScopePerson {
			return fmt.Errorf("%w: a GitHub user connection belongs to a person", ErrInvalid)
		}
		if c.KeyRef != "" || c.TokenRef != "" || c.AppID != "" || c.InstallationID != 0 {
			return fmt.Errorf("%w: a GitHub user connection keeps its token in the user-token store, not in the record", ErrInvalid)
		}
	case KindAWS, KindGCP:
		return c.validateCloud()
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalid, c.Kind)
	}
	if c.AccessKeyID != "" || c.RoleARN != "" || c.Region != "" {
		return fmt.Errorf("%w: only an AWS connection has an access key ID, role or region", ErrInvalid)
	}
	return nil
}

// validateCloud checks a KindAWS or KindGCP connection. Both sign in as the server, have no
// GitHub fields, and keep any key behind a secret reference.
func (c Connection) validateCloud() error {
	if c.Scope != ScopeServer {
		return fmt.Errorf("%w: an %s connection signs in as the server", ErrInvalid, c.Kind)
	}
	if c.BaseURL != "" || c.AppID != "" || c.InstallationID != 0 || c.TokenRef != "" {
		return fmt.Errorf("%w: an %s connection has no base URL, app fields or token_ref", ErrInvalid, c.Kind)
	}
	if c.KeyRef != "" {
		if err := secretRef("key_ref", c.KeyRef); err != nil {
			return err
		}
	}
	if c.Kind == KindGCP {
		if c.AccessKeyID != "" || c.RoleARN != "" || c.Region != "" {
			return fmt.Errorf("%w: only an AWS connection has an access key ID, role or region", ErrInvalid)
		}
		return nil
	}
	if (c.AccessKeyID == "") != (c.KeyRef == "") {
		return fmt.Errorf("%w: an AWS key pair needs both access_key_id and key_ref, or neither for the default chain", ErrInvalid)
	}
	if c.RoleARN != "" && !strings.HasPrefix(c.RoleARN, "arn:") {
		return fmt.Errorf("%w: role_arn must be an ARN", ErrInvalid)
	}
	return nil
}

func secretRef(field, ref string) error {
	if ref == "" {
		return fmt.Errorf("%w: %s is required", ErrInvalid, field)
	}
	if _, err := outbound.ParseSecretRef(ref); err != nil {
		return fmt.Errorf("%w: %s must be a secret reference such as env:NAME or file:/abs/path", ErrInvalid, field)
	}
	return nil
}
