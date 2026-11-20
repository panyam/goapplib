package github_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/github"
	"github.com/panyam/goapplib/storage/mounts/github/githubtest"
)

var secret = []byte("s3cret")

func pushBody(ref, before, after string, deleted bool) []byte {
	return []byte(`{"ref":"` + ref + `","before":"` + before + `","after":"` + after + `","deleted":` +
		map[bool]string{true: "true", false: "false"}[deleted] +
		`,"repository":{"full_name":"acme/board","default_branch":"main"}}`)
}

func TestAPushNamesItsSourceBranchAndCommit(t *testing.T) {
	e := newEnv(t, false)
	before := e.repo.Head("main")
	after := e.repo.Commit("main", map[string]string{"x": "1"}, nil)

	p, err := github.ParsePush(e.repo.PushRequest("/hook", secret, "main"), secret)
	if err != nil {
		t.Fatal(err)
	}
	want := mounts.Change{Source: "github://acme/board", Ref: "main", RefKind: mounts.RefBranch, Version: after, Before: before, Default: true}
	if p.Change != want || p.Delivery == "" {
		t.Errorf("ParsePush = %+v; want %+v and a delivery id", p, want)
	}

	side := e.repo.Commit("side", map[string]string{"y": "1"}, nil)
	if p, err := github.ParsePush(e.repo.PushRequest("/hook", secret, "side"), secret); err != nil || p.Ref != "side" || p.Default || p.Version != side {
		t.Errorf("a push to side = %+v, %v; want side at %s, not the default", p.Change, err, side)
	}
}

func TestAPushToATagOrADeletedBranch(t *testing.T) {
	sha := strings.Repeat("a", 40)
	zero := strings.Repeat("0", 40)
	p, err := github.ParsePush(githubtest.SignedRequest("/hook", secret, "push", pushBody("refs/tags/v1.2.0", zero, sha, false)), secret)
	if err != nil || p.RefKind != mounts.RefTag || p.Ref != "v1.2.0" || p.Version != sha || p.Before != "" || p.Default {
		t.Errorf("a tag push = %+v, %v; want tag v1.2.0 at %s with no before", p.Change, err, sha)
	}
	p, err = github.ParsePush(githubtest.SignedRequest("/hook", secret, "push", pushBody("refs/heads/old", sha, zero, true)), secret)
	if err != nil || !p.Deleted || p.Version != "" || p.Before != sha || p.Ref != "old" {
		t.Errorf("a branch deletion = %+v, %v; want Deleted, no version, before %s", p.Change, err, sha)
	}
	if _, err := github.ParsePush(githubtest.SignedRequest("/hook", secret, "push", pushBody("refs/notes/x", zero, sha, false)), secret); !errors.Is(err, github.ErrNotPush) {
		t.Errorf("a push to refs/notes = %v, want ErrNotPush", err)
	}
}

func TestAWebhookIsRefusedUnlessSignedWithTheSecret(t *testing.T) {
	body := pushBody("refs/heads/main", "", strings.Repeat("a", 40), false)

	req := githubtest.SignedRequest("/hook", secret, "push", body)
	if _, err := github.ParsePush(req, []byte("wrong")); !errors.Is(err, github.ErrBadSignature) {
		t.Errorf("a wrong secret = %v, want ErrBadSignature", err)
	}

	req = githubtest.SignedRequest("/hook", secret, "push", body)
	changed := bytes.Replace(body, []byte("acme/board"), []byte("evil/board"), 1)
	req.Body = io.NopCloser(bytes.NewReader(changed))
	if _, err := github.ParsePush(req, secret); !errors.Is(err, github.ErrBadSignature) {
		t.Errorf("a body changed after signing = %v, want ErrBadSignature", err)
	}

	for _, sig := range []string{"", "sha1=abcd", "sha256=zz"} {
		req = githubtest.SignedRequest("/hook", secret, "push", body)
		req.Header.Set("X-Hub-Signature-256", sig)
		if _, err := github.ParsePush(req, secret); !errors.Is(err, github.ErrBadSignature) {
			t.Errorf("signature %q = %v, want ErrBadSignature", sig, err)
		}
	}
	if _, err := github.ParsePush(githubtest.SignedRequest("/hook", nil, "push", body), nil); !errors.Is(err, github.ErrBadSignature) {
		t.Errorf("an empty secret = %v, want ErrBadSignature", err)
	}
}

func TestOtherWebhookEvents(t *testing.T) {
	if _, err := github.ParsePush(githubtest.SignedRequest("/hook", secret, "ping", []byte(`{"zen":"hi"}`)), secret); !errors.Is(err, github.ErrPing) {
		t.Errorf("a ping = %v, want ErrPing", err)
	}
	if _, err := github.ParsePush(githubtest.SignedRequest("/hook", secret, "issues", []byte(`{}`)), secret); !errors.Is(err, github.ErrNotPush) {
		t.Errorf("an issues event = %v, want ErrNotPush", err)
	}
	unsigned := githubtest.SignedRequest("/hook", []byte("other"), "ping", []byte(`{}`))
	if _, err := github.ParsePush(unsigned, secret); !errors.Is(err, github.ErrBadSignature) {
		t.Errorf("a ping signed with another secret = %v, want ErrBadSignature, checked before the event", err)
	}
}

func TestAFormEncodedPush(t *testing.T) {
	sha := strings.Repeat("b", 40)
	form := []byte(url.Values{"payload": {string(pushBody("refs/heads/main", "", sha, false))}}.Encode())
	req := githubtest.SignedRequest("/hook", secret, "push", form)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if p, err := github.ParsePush(req, secret); err != nil || p.Version != sha || p.Ref != "main" {
		t.Errorf("a form-encoded push = %+v, %v; want main at %s", p.Change, err, sha)
	}
}

func TestAnOversizedWebhookBody(t *testing.T) {
	big := make([]byte, github.MaxWebhookBytes+1)
	req := httptest.NewRequest(http.MethodPost, "/hook", bytes.NewReader(big))
	req.Header.Set("X-GitHub-Event", "push")
	if _, err := github.ParsePush(req, secret); !errors.Is(err, mounts.ErrTooLarge) {
		t.Errorf("a body over the cap = %v, want ErrTooLarge", err)
	}
}
