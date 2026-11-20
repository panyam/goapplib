package connections

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestValidateAcceptsEachKind(t *testing.T) {
	for _, c := range []Connection{
		{ID: "acme", Kind: KindGitHubApp, Scope: ScopeServer, AppID: "12345", InstallationID: 1, KeyRef: "file:/run/secrets/gh-app.pem"},
		{ID: "ci-pat", Kind: KindGitHubToken, Scope: ScopeServer, TokenRef: "env:GITHUB_TOKEN"},
		{ID: "alice-pat", Kind: KindGitHubToken, Scope: ScopePerson, Owner: "alice", TokenRef: "env:ALICE_TOKEN"},
		{ID: "alice-gh", Kind: KindGitHubUser, Scope: ScopePerson, Owner: "alice"},
		{ID: "alice-paste", Kind: KindGitHubUser, Scope: ScopePerson, Owner: "alice", Pasted: true},
	} {
		if err := c.Validate(); err != nil {
			t.Errorf("%s: %v", c.ID, err)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	const raw = "ghp_0123456789abcdefghijklmnopqrstuvwxyz"
	for name, c := range map[string]Connection{
		"bad id":                 {ID: "Acme", Kind: KindGitHubToken, Scope: ScopeServer, TokenRef: "env:T"},
		"unknown kind":           {ID: "x", Kind: "gitlab", Scope: ScopeServer},
		"unknown scope":          {ID: "x", Kind: KindGitHubToken, TokenRef: "env:T"},
		"server with owner":      {ID: "x", Kind: KindGitHubToken, Scope: ScopeServer, Owner: "alice", TokenRef: "env:T"},
		"person without owner":   {ID: "x", Kind: KindGitHubUser, Scope: ScopePerson},
		"app for a person":       {ID: "x", Kind: KindGitHubApp, Scope: ScopePerson, Owner: "alice", AppID: "1", InstallationID: 1, KeyRef: "env:K"},
		"app without install":    {ID: "x", Kind: KindGitHubApp, Scope: ScopeServer, AppID: "1", KeyRef: "env:K"},
		"app without key":        {ID: "x", Kind: KindGitHubApp, Scope: ScopeServer, AppID: "1", InstallationID: 1},
		"pasted on a token kind": {ID: "x", Kind: KindGitHubToken, Scope: ScopeServer, TokenRef: "env:T", Pasted: true},
		"raw token in token_ref": {ID: "x", Kind: KindGitHubToken, Scope: ScopeServer, TokenRef: raw},
		"relative file ref":      {ID: "x", Kind: KindGitHubToken, Scope: ScopeServer, TokenRef: "file:secrets/t"},
		"user with token_ref":    {ID: "x", Kind: KindGitHubUser, Scope: ScopePerson, Owner: "alice", TokenRef: "env:T"},
		"server user connection": {ID: "x", Kind: KindGitHubUser, Scope: ScopeServer},
	} {
		err := c.Validate()
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: Validate() = %v, want ErrInvalid", name, err)
			continue
		}
		if strings.Contains(err.Error(), raw) {
			t.Errorf("%s: the error repeats the secret: %v", name, err)
		}
	}
}

func TestViewer(t *testing.T) {
	ctx := context.Background()
	if v := Viewer(ctx); v != "" {
		t.Errorf("Viewer of a bare context = %q, want empty", v)
	}
	if v := Viewer(WithViewer(ctx, "alice")); v != "alice" {
		t.Errorf("Viewer = %q, want alice", v)
	}
}
