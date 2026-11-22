package github_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/fsstore"
	"github.com/panyam/goapplib/storage/mounts/github"
)

func TestEnsureWebhookMakesThenUpdatesOneHook(t *testing.T) {
	e := newEnv(t, true)
	e.gh.AddToken("tok-admin")
	req := mounts.WebhookRequest{URL: "https://app.example/hooks/github", Secret: []byte("s1"), Events: []string{"push"}}
	h, err := e.drv.EnsureWebhook(e.ctx, e.repo.Source(), req, static("tok-admin"))
	if err != nil {
		t.Fatal(err)
	}
	if !h.Created || h.ID == "" || h.URL != req.URL || !slices.Equal(h.Events, []string{"push"}) {
		t.Errorf("first EnsureWebhook = %+v, want a created push hook", h)
	}
	req.Secret, req.Events = []byte("s2"), []string{"push", "create"}
	h2, err := e.drv.EnsureWebhook(e.ctx, e.repo.Source(), req, static("tok-admin"))
	if err != nil {
		t.Fatal(err)
	}
	if h2.Created || h2.ID != h.ID || !slices.Equal(h2.Events, req.Events) {
		t.Errorf("second EnsureWebhook = %+v, want hook %s updated", h2, h.ID)
	}
	hooks := e.repo.Hooks()
	if len(hooks) != 1 {
		t.Fatalf("hooks = %+v, want one", hooks)
	}
	got := hooks[0]
	if got.Secret != "s2" || got.ContentType != "json" || !got.Active || got.Token != "tok-admin" || !slices.Equal(got.Events, req.Events) {
		t.Errorf("hook = %+v, want secret s2, json, active, made with tok-admin, events %v", got, req.Events)
	}
}

func TestWebhookWithoutAdminIsPermissionDenied(t *testing.T) {
	e := newEnv(t, false)
	e.repo.HookManagers("tok-admin")
	e.gh.AddToken("tok-reader")
	req := mounts.WebhookRequest{URL: "https://app.example/h", Secret: []byte("s"), Events: []string{"push"}}
	_, err := e.drv.EnsureWebhook(e.ctx, e.repo.Source(), req, static("tok-reader"))
	if !errors.Is(err, mounts.ErrPermissionDenied) || !strings.Contains(err.Error(), "admin") {
		t.Errorf("EnsureWebhook with a reader's token = %v, want ErrPermissionDenied naming admin rights", err)
	}
	if err := e.drv.DeleteWebhook(e.ctx, e.repo.Source(), req.URL, static("tok-reader")); !errors.Is(err, mounts.ErrPermissionDenied) {
		t.Errorf("DeleteWebhook with a reader's token = %v, want ErrPermissionDenied", err)
	}
	if _, err := e.drv.EnsureWebhook(e.ctx, e.repo.Source(), req, static("tok-admin")); err != nil {
		t.Errorf("EnsureWebhook with a manager's token = %v", err)
	}
}

func TestEnsureWebhookURLGitHubRefusesIsInvalid(t *testing.T) {
	e := newEnv(t, false)
	e.gh.AddToken("tok")
	_, err := e.drv.EnsureWebhook(e.ctx, e.repo.Source(), mounts.WebhookRequest{URL: "not a url", Secret: []byte("s"), Events: []string{"push"}}, static("tok"))
	if !errors.Is(err, mounts.ErrInvalidArgument) || !strings.Contains(err.Error(), "url") {
		t.Errorf("EnsureWebhook with a bad URL = %v, want ErrInvalidArgument with GitHub's message", err)
	}
}

func TestDeleteWebhookRemovesOnlyItsURLAndIsIdempotent(t *testing.T) {
	e := newEnv(t, false)
	e.gh.AddToken("tok")
	for _, u := range []string{"https://app.example/a", "https://app.example/b"} {
		if _, err := e.drv.EnsureWebhook(e.ctx, e.repo.Source(), mounts.WebhookRequest{URL: u, Secret: []byte("s"), Events: []string{"push"}}, static("tok")); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := e.drv.DeleteWebhook(e.ctx, e.repo.Source(), "https://app.example/a", static("tok")); err != nil {
			t.Fatal(err)
		}
	}
	if hooks := e.repo.Hooks(); len(hooks) != 1 || hooks[0].URL != "https://app.example/b" {
		t.Errorf("hooks after deleting a = %+v, want only b", hooks)
	}
}

// The whole loop an app runs: make the hook through the registry, push, verify the delivery, and
// find the mounts to sync.
func TestARegistryWebhookDeliversAVerifiedPush(t *testing.T) {
	e := newEnv(t, true)
	e.repo.Grant("tok")
	secret := []byte("hook-secret")
	var mu sync.Mutex
	var got []github.Push
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := github.ParsePush(r, secret)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		got = append(got, p)
		mu.Unlock()
	}))
	defer app.Close()

	store, err := fsstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reg := mounts.NewRegistry(store, []mounts.Driver{e.drv}, mounts.WithCredentials(func(context.Context, string, string) (mounts.Credentials, error) {
		return static("tok"), nil
	}))
	if _, err := reg.Create(e.ctx, mounts.Mount{Name: "board", Kind: mounts.KindGitHub, Source: e.repo.Source(), Mode: mounts.ModeLive, Connection: "c"}); err != nil {
		t.Fatal(err)
	}
	if _, err := reg.EnsureWebhook(e.ctx, "board", mounts.WebhookRequest{URL: app.URL + "/hooks/github", Secret: secret}); err != nil {
		t.Fatal(err)
	}
	sha, err := e.repo.Push("main", map[string]string{"README.md": "v2"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 1 || got[0].Version != sha {
		t.Fatalf("the app got %+v, want one push of %s", got, sha)
	}
	ms, err := reg.Following(e.ctx, mounts.KindGitHub, got[0].Change)
	if err != nil || len(ms) != 1 || ms[0].Name != "board" {
		t.Errorf("Following = %v, %v; want board", ms, err)
	}
}

func TestPushReportsAHookThatRefuses(t *testing.T) {
	e := newEnv(t, false)
	e.gh.AddToken("tok")
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := github.ParsePush(r, []byte("right")); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
		}
	}))
	defer app.Close()
	if _, err := e.drv.EnsureWebhook(e.ctx, e.repo.Source(), mounts.WebhookRequest{URL: app.URL, Secret: []byte("wrong"), Events: []string{"push"}}, static("tok")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.repo.Push("main", map[string]string{"x": "1"}, nil); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("Push to a hook that refuses = %v, want an error naming 401", err)
	}
}
