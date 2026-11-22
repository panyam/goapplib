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
	if c.Sealed && (c.Scope != ScopeServer || c.Kind.holdsUserToken()) {
		return fmt.Errorf("%w: only a server connection has a sealed secret", ErrInvalid)
	}
	switch c.Kind {
	case KindGitHubApp:
		if c.Scope != ScopeServer {
			return fmt.Errorf("%w: a GitHub App installation signs in as the server", ErrInvalid)
		}
		if c.AppID == "" || c.InstallationID <= 0 {
			return fmt.Errorf("%w: a GitHub App connection needs an app ID and an installation ID", ErrInvalid)
		}
		if err := c.secretField("key_ref", c.KeyRef); err != nil {
			return err
		}
		if c.TokenRef != "" {
			return fmt.Errorf("%w: a GitHub App connection has no token_ref", ErrInvalid)
		}
	case KindGitHubToken:
		if err := c.secretField("token_ref", c.TokenRef); err != nil {
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
		if err := c.noEntraFields(); err != nil {
			return err
		}
		return c.validateCloud()
	case KindEntra, KindEntraUser:
		return c.validateEntra()
	default:
		return fmt.Errorf("%w: unknown kind %q", ErrInvalid, c.Kind)
	}
	if c.AccessKeyID != "" || c.RoleARN != "" || c.Region != "" {
		return fmt.Errorf("%w: only an AWS connection has an access key ID, role or region", ErrInvalid)
	}
	return c.noEntraFields()
}

func (c Connection) noEntraFields() error {
	if c.TenantID != "" || c.ClientID != "" || c.EntraAuth != "" || c.CertRef != "" {
		return fmt.Errorf("%w: only an Entra connection has a tenant, client ID, entra_auth or cert_ref", ErrInvalid)
	}
	return nil
}

// validateEntra checks a KindEntra or KindEntraUser connection. Both name a tenant and, except
// for a system-assigned managed identity, an app registration. Neither has GitHub or AWS fields.
func (c Connection) validateEntra() error {
	if c.AppID != "" || c.InstallationID != 0 || c.TokenRef != "" || c.AccessKeyID != "" || c.RoleARN != "" || c.Region != "" {
		return fmt.Errorf("%w: an %s connection has no GitHub or AWS fields", ErrInvalid, c.Kind)
	}
	if c.BaseURL != "" && !strings.HasPrefix(c.BaseURL, "https://") && !strings.HasPrefix(c.BaseURL, "http://127.0.0.1") && !strings.HasPrefix(c.BaseURL, "http://localhost") {
		return fmt.Errorf("%w: an Entra authority must be https", ErrInvalid)
	}
	if c.Kind == KindEntraUser {
		if c.Scope != ScopePerson {
			return fmt.Errorf("%w: an Entra user connection belongs to a person", ErrInvalid)
		}
		if c.TenantID == "" || c.ClientID == "" {
			return fmt.Errorf("%w: an Entra user connection needs a tenant and a client ID", ErrInvalid)
		}
		if c.EntraAuth != "" || c.CertRef != "" {
			return fmt.Errorf("%w: an Entra user connection has no entra_auth or cert_ref", ErrInvalid)
		}
		if c.KeyRef != "" {
			return secretRef("key_ref", c.KeyRef)
		}
		return nil
	}
	if c.Scope != ScopeServer {
		return fmt.Errorf("%w: an Entra app connection signs in as the server", ErrInvalid)
	}
	switch c.EntraAuth {
	case EntraSecret, EntraCertificate:
		if c.TenantID == "" || c.ClientID == "" {
			return fmt.Errorf("%w: an Entra %s connection needs a tenant and a client ID", ErrInvalid, c.EntraAuth)
		}
		if err := c.secretField("key_ref", c.KeyRef); err != nil {
			return err
		}
		if c.EntraAuth == EntraCertificate {
			return secretRef("cert_ref", c.CertRef)
		}
		if c.CertRef != "" {
			return fmt.Errorf("%w: an Entra secret connection has no cert_ref", ErrInvalid)
		}
	case EntraManagedIdentity:
		if c.Sealed {
			return fmt.Errorf("%w: a managed identity has no secret to seal", ErrInvalid)
		}
		if c.KeyRef != "" || c.CertRef != "" {
			return fmt.Errorf("%w: a managed identity holds no key_ref or cert_ref", ErrInvalid)
		}
	case EntraWorkloadIdentity:
		if c.Sealed {
			return fmt.Errorf("%w: a workload identity reads a file the cluster rotates, so it can't be sealed", ErrInvalid)
		}
		if c.TenantID == "" || c.ClientID == "" {
			return fmt.Errorf("%w: an Entra workload identity connection needs a tenant and a client ID", ErrInvalid)
		}
		if !strings.HasPrefix(c.KeyRef, "file:") {
			return fmt.Errorf("%w: a workload identity's key_ref must be a file: reference to its token", ErrInvalid)
		}
		if err := secretRef("key_ref", c.KeyRef); err != nil {
			return err
		}
		if c.CertRef != "" {
			return fmt.Errorf("%w: a workload identity has no cert_ref", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: entra_auth must be secret, certificate, managed-identity or workload-identity", ErrInvalid)
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
	if c.KeyRef != "" || c.Sealed {
		if err := c.secretField("key_ref", c.KeyRef); err != nil {
			return err
		}
	}
	if c.Kind == KindGCP {
		if c.AccessKeyID != "" || c.RoleARN != "" || c.Region != "" {
			return fmt.Errorf("%w: only an AWS connection has an access key ID, role or region", ErrInvalid)
		}
		return nil
	}
	if (c.AccessKeyID == "") != (c.KeyRef == "" && !c.Sealed) {
		return fmt.Errorf("%w: an AWS key pair needs both access_key_id and key_ref, or neither for the default chain", ErrInvalid)
	}
	if c.RoleARN != "" && !strings.HasPrefix(c.RoleARN, "arn:") {
		return fmt.Errorf("%w: role_arn must be an ARN", ErrInvalid)
	}
	return nil
}

// secretField checks a field that holds a secret reference, or that stays empty when c is Sealed.
func (c Connection) secretField(field, ref string) error {
	if c.Sealed {
		if ref != "" {
			return fmt.Errorf("%w: a sealed connection has no %s", ErrInvalid, field)
		}
		return nil
	}
	return secretRef(field, ref)
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
