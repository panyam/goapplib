package mounts_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/folder"
	"github.com/panyam/goapplib/storage/mounts/fsstore"
	"golang.org/x/oauth2"
)

// hookGitHub is fakeGitHub with webhooks, recording each request and the token it came with.
type hookGitHub struct {
	fakeGitHub
	ensured []mounts.WebhookRequest
	deleted []string
}

func (h *hookGitHub) EnsureWebhook(_ context.Context, source string, req mounts.WebhookRequest, ts oauth2.TokenSource) (mounts.Webhook, error) {
	h.note(ts)
	h.ensured = append(h.ensured, req)
	return mounts.Webhook{ID: "1", URL: req.URL, Events: req.Events, Created: true}, nil
}

func (h *hookGitHub) DeleteWebhook(_ context.Context, source, url string, ts oauth2.TokenSource) error {
	h.note(ts)
	h.deleted = append(h.deleted, url)
	return nil
}

func hookRegistry(t *testing.T, gh mounts.Driver) (*mounts.Registry, context.Context) {
	t.Helper()
	store, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := mounts.NewRegistry(store, []mounts.Driver{folder.New(), gh},
		mounts.WithFixed(mounts.Mount{Name: "notes", Kind: mounts.KindFolder, Source: t.TempDir(), Mode: mounts.ModeLive}),
		mounts.WithCredentials(func(_ context.Context, _, conn string) (oauth2.TokenSource, error) {
			return oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "tok-" + conn}), nil
		}))
	ctx := context.Background()
	for _, m := range []mounts.Mount{
		{Name: "boards", Kind: mounts.KindGitHub, Source: "github://acme/boards", Connection: "acme"},
		{Name: "public", Kind: mounts.KindGitHub, Source: "github://acme/boards"},
	} {
		if _, err := reg.Create(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	return reg, ctx
}

func TestEnsureWebhookGoesThroughTheMountsConnection(t *testing.T) {
	gh := &hookGitHub{}
	reg, ctx := hookRegistry(t, gh)
	gh.tokens = nil
	h, err := reg.EnsureWebhook(ctx, "boards", mounts.WebhookRequest{URL: "https://app.example/hooks/github", Secret: []byte("s")})
	if err != nil {
		t.Fatal(err)
	}
	if !h.Created || h.URL != "https://app.example/hooks/github" {
		t.Errorf("EnsureWebhook = %+v", h)
	}
	if len(gh.ensured) != 1 || !slices.Equal(gh.ensured[0].Events, []string{"push"}) {
		t.Errorf("the driver got %+v, want one request for push", gh.ensured)
	}
	if err := reg.DeleteWebhook(ctx, "boards", "https://app.example/hooks/github"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(gh.deleted, []string{"https://app.example/hooks/github"}) {
		t.Errorf("deleted = %v", gh.deleted)
	}
	if !slices.Equal(gh.tokens, []string{"tok-acme", "tok-acme"}) {
		t.Errorf("driver tokens = %v, want the connection's token on both calls", gh.tokens)
	}
}

func TestEnsureWebhookRefusals(t *testing.T) {
	gh := &hookGitHub{}
	reg, ctx := hookRegistry(t, gh)
	ok := mounts.WebhookRequest{URL: "https://app.example/h", Secret: []byte("s")}
	for name, tc := range map[string]struct {
		mount string
		req   mounts.WebhookRequest
	}{
		"relative URL":       {"boards", mounts.WebhookRequest{URL: "/hooks", Secret: []byte("s")}},
		"ftp URL":            {"boards", mounts.WebhookRequest{URL: "ftp://app.example/h", Secret: []byte("s")}},
		"no secret":          {"boards", mounts.WebhookRequest{URL: "https://app.example/h"}},
		"no connection":      {"public", ok},
		"kind without hooks": {"notes", ok},
	} {
		if _, err := reg.EnsureWebhook(ctx, tc.mount, tc.req); !errors.Is(err, mounts.ErrInvalidArgument) {
			t.Errorf("%s: EnsureWebhook = %v, want ErrInvalidArgument", name, err)
		}
	}
	if _, err := reg.EnsureWebhook(ctx, "nope", ok); !errors.Is(err, mounts.ErrNotFound) {
		t.Errorf("unknown mount: %v, want ErrNotFound", err)
	}
	for name, m := range map[string]string{"no connection": "public", "kind without hooks": "notes"} {
		if err := reg.DeleteWebhook(ctx, m, ok.URL); !errors.Is(err, mounts.ErrInvalidArgument) {
			t.Errorf("%s: DeleteWebhook = %v, want ErrInvalidArgument", name, err)
		}
	}
	if len(gh.ensured)+len(gh.deleted) != 0 {
		t.Errorf("the driver was called for a refused request: %v %v", gh.ensured, gh.deleted)
	}
}

func TestAGitHubMountWithoutHooksSupportIsRefused(t *testing.T) {
	reg, ctx := hookRegistry(t, &fakeGitHub{})
	if _, err := reg.EnsureWebhook(ctx, "boards", mounts.WebhookRequest{URL: "https://app.example/h", Secret: []byte("s")}); !errors.Is(err, mounts.ErrInvalidArgument) {
		t.Errorf("EnsureWebhook through a driver with no WebhookManager = %v, want ErrInvalidArgument", err)
	}
}
