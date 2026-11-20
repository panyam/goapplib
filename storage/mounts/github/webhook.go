package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/panyam/goapplib/storage/mounts"
)

// MaxWebhookBytes caps the body ParsePush reads. GitHub caps a webhook payload at 25 MB.
const MaxWebhookBytes = 25 << 20

var (
	// ErrBadSignature means a webhook delivery's X-Hub-Signature-256 is missing, malformed, or
	// isn't the HMAC-SHA256 of the body under the secret. A handler answers 401.
	ErrBadSignature = errors.New("github: bad webhook signature")

	// ErrPing means the delivery is the ping GitHub sends when a webhook is made. Its signature
	// checked out. A handler answers 200.
	ErrPing = errors.New("github: webhook ping")

	// ErrNotPush means the delivery is a signed event other than a push or a ping. A handler
	// answers 200 or 204 and ignores it.
	ErrNotPush = errors.New("github: webhook event isn't a push")
)

// Push is one push event a GitHub webhook sent. Change is the push in the form every driver
// shares, for Registry.Following. Delivery is GitHub's id for the delivery, the same on a
// redelivery, so an app can drop a duplicate.
type Push struct {
	mounts.Change
	Delivery string
}

// ParsePush checks a GitHub webhook delivery's signature and reads the push it carries (goapplib
// issue 179). secret is the webhook's secret, which the app resolves from a secret reference, as
// connections do. An empty secret refuses every delivery, since anyone could sign under it.
//
// It reads at most MaxWebhookBytes of the body, and checks X-Hub-Signature-256 as an HMAC-SHA256
// of the raw body in constant time before reading anything from it. A body sent as
// application/x-www-form-urlencoded carries the JSON in its payload field, as GitHub sends it when
// the webhook's content type is set that way.
//
// It returns ErrBadSignature for a missing or wrong signature, ErrPing for a ping, and ErrNotPush
// for any other event. A push's Source is "github://owner/repo", its Ref the branch or tag name
// with "refs/heads/" or "refs/tags/" taken off, and its Version the new commit. A deleted ref has
// Deleted set and no Version. A push to anything but a branch or a tag returns ErrNotPush.
func ParsePush(r *http.Request, secret []byte) (Push, error) {
	if len(secret) == 0 {
		return Push{}, fmt.Errorf("%w: the webhook has no secret", ErrBadSignature)
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxWebhookBytes+1))
	if err != nil {
		return Push{}, fmt.Errorf("%w: reading the webhook body: %v", mounts.ErrUnavailable, err)
	}
	if len(body) > MaxWebhookBytes {
		return Push{}, fmt.Errorf("%w: the webhook body is over %d bytes", mounts.ErrTooLarge, MaxWebhookBytes)
	}
	sig, ok := strings.CutPrefix(r.Header.Get("X-Hub-Signature-256"), "sha256=")
	got, err := hex.DecodeString(sig)
	if !ok || err != nil {
		return Push{}, fmt.Errorf("%w: no sha256 signature", ErrBadSignature)
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	if !hmac.Equal(got, mac.Sum(nil)) {
		return Push{}, ErrBadSignature
	}

	switch ev := r.Header.Get("X-GitHub-Event"); ev {
	case "push":
	case "ping":
		return Push{}, ErrPing
	default:
		return Push{}, fmt.Errorf("%w: %q", ErrNotPush, ev)
	}
	if ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); ct == "application/x-www-form-urlencoded" {
		form, err := url.ParseQuery(string(body))
		if err != nil {
			return Push{}, fmt.Errorf("%w: the webhook form: %v", mounts.ErrInvalidArgument, err)
		}
		body = []byte(form.Get("payload"))
	}
	var p struct {
		Ref        string `json:"ref"`
		Before     string `json:"before"`
		After      string `json:"after"`
		Deleted    bool   `json:"deleted"`
		Repository struct {
			FullName      string `json:"full_name"`
			DefaultBranch string `json:"default_branch"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return Push{}, fmt.Errorf("%w: the push payload: %v", mounts.ErrInvalidArgument, err)
	}
	source := "github://" + p.Repository.FullName
	if _, err := repo(source); err != nil {
		return Push{}, err
	}
	c := mounts.Change{Source: source}
	if b, ok := strings.CutPrefix(p.Ref, "refs/heads/"); ok {
		c.Ref, c.RefKind, c.Default = b, mounts.RefBranch, b == p.Repository.DefaultBranch
	} else if t, ok := strings.CutPrefix(p.Ref, "refs/tags/"); ok {
		c.Ref, c.RefKind = t, mounts.RefTag
	} else {
		return Push{}, fmt.Errorf("%w: a push to %q", ErrNotPush, p.Ref)
	}
	c.Deleted = p.Deleted
	if !p.Deleted {
		if !commitSHA.MatchString(p.After) {
			return Push{}, fmt.Errorf("%w: the push's after %q isn't a commit", mounts.ErrInvalidArgument, p.After)
		}
		c.Version = p.After
	}
	if commitSHA.MatchString(p.Before) && strings.Trim(p.Before, "0") != "" {
		c.Before = p.Before
	}
	return Push{Change: c, Delivery: r.Header.Get("X-GitHub-Delivery")}, nil
}
