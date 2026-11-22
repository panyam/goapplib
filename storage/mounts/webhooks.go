package mounts

import (
	"context"
	"fmt"
	"net/url"
)

// WebhookManager is a Driver that can make a source send a webhook to the app on each change, so
// the app syncs a mount when its branch moves rather than polling it (goapplib issue 185). A hook
// is named by the URL it sends to, so making one twice updates the first rather than adding a
// second.
type WebhookManager interface {
	Driver

	// EnsureWebhook makes a hook on source that sends req.Events to req.URL, signed with
	// req.Secret, or updates the hook already sending to req.URL. A token that can't manage the
	// source's hooks returns ErrPermissionDenied, and a request the source refuses, such as a URL
	// it won't send to, returns ErrInvalidArgument.
	EnsureWebhook(ctx context.Context, source string, req WebhookRequest, creds Credentials) (Webhook, error)

	// DeleteWebhook removes the hooks on source that send to url. No such hook is not an error,
	// so deleting twice is safe. A token that can't manage the source's hooks returns
	// ErrPermissionDenied.
	DeleteWebhook(ctx context.Context, source, url string, creds Credentials) error
}

// WebhookRequest describes a hook. URL is where the source sends each delivery, and must be an
// absolute http or https URL. Secret signs each delivery, and must not be empty, since the app
// can't verify an unsigned one. Events names the source's events, and empty means "push".
type WebhookRequest struct {
	URL    string
	Secret []byte
	Events []string
}

// Webhook is a hook as the source holds it. ID is the source's id for it. Created is true when
// EnsureWebhook made it, and false when it updated a hook already sending to URL.
type Webhook struct {
	ID      string
	URL     string
	Events  []string
	Created bool
}

// EnsureWebhook makes or updates the hook that sends the source's changes of the mount named name
// to req.URL, through the mount's connection. It returns ErrInvalidArgument for a URL that isn't
// absolute http or https, an empty secret, a mount with no connection (a source doesn't let an
// anonymous caller manage hooks), or a kind whose driver has no WebhookManager.
func (r *Registry) EnsureWebhook(ctx context.Context, name string, req WebhookRequest) (Webhook, error) {
	if err := checkHookURL(req.URL); err != nil {
		return Webhook{}, err
	}
	if len(req.Secret) == 0 {
		return Webhook{}, fmt.Errorf("%w: a webhook needs a secret, or its deliveries can't be verified", ErrInvalidArgument)
	}
	if len(req.Events) == 0 {
		req.Events = []string{"push"}
	}
	wm, m, creds, err := r.webhooks(ctx, name)
	if err != nil {
		return Webhook{}, err
	}
	return wm.EnsureWebhook(ctx, m.Source, req, creds)
}

// DeleteWebhook removes the hooks that send the source's changes of the mount named name to url,
// through the mount's connection. No such hook is not an error. It returns ErrInvalidArgument in
// the cases EnsureWebhook does, apart from the secret.
func (r *Registry) DeleteWebhook(ctx context.Context, name, url string) error {
	if err := checkHookURL(url); err != nil {
		return err
	}
	wm, m, creds, err := r.webhooks(ctx, name)
	if err != nil {
		return err
	}
	return wm.DeleteWebhook(ctx, m.Source, url, creds)
}

func (r *Registry) webhooks(ctx context.Context, name string) (WebhookManager, Mount, Credentials, error) {
	m, err := r.Get(ctx, name)
	if err != nil {
		return nil, Mount{}, nil, err
	}
	d, err := r.driver(m)
	if err != nil {
		return nil, Mount{}, nil, err
	}
	wm, ok := d.(WebhookManager)
	if !ok {
		return nil, Mount{}, nil, fmt.Errorf("%w: %s mounts have no webhooks", ErrInvalidArgument, m.Kind)
	}
	if m.Connection == "" {
		return nil, Mount{}, nil, fmt.Errorf("%w: mount %q has no connection, and managing a webhook needs a token", ErrInvalidArgument, m.Name)
	}
	creds, err := r.credentials(ctx, m)
	if err != nil {
		return nil, Mount{}, nil, err
	}
	return wm, m, creds, nil
}

func checkHookURL(s string) error {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%w: a webhook URL must be an absolute http or https URL", ErrInvalidArgument)
	}
	return nil
}
