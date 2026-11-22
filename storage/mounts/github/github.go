// Package github is the mounts driver for GitHub repositories, read and written through GitHub's
// REST API. A mount's Source is "github://owner/repo".
//
// A snapshot mount pins a commit. When the mount is made, the driver downloads the commit's
// tarball into a blob store and writes a manifest, so the mount serves the same bytes for its whole
// life, and after a restart, without asking GitHub again. A live mount reads a branch through the
// Contents API, and each Put or Delete is one commit on it.
//
// The driver sends each request with the bearer token of the mounts.BearerCredentials the registry
// hands it, which come from the mount's connection. Other kinds of credentials return
// ErrInvalidArgument. A private repo answers 404 to a request without a valid token, so the driver
// reports ErrNotFound for it, as GitHub intends.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
)

// DefaultBaseURL is GitHub's API root. GitHub Enterprise and tests set another with WithBaseURL.
const DefaultBaseURL = "https://api.github.com"

// Driver serves mounts.KindGitHub mounts. It implements mounts.Preparer, mounts.Writer,
// mounts.Localizer and mounts.VersionReader. Share one Driver per process, since it caches manifests and builds each
// snapshot's folder view once.
type Driver struct {
	blobs  blobs.Store
	base   string
	client *http.Client
	limits mounts.Limits
	views  string

	rawURL string
	lfsURL string

	mu        sync.Mutex
	manifests map[blobs.Hash]*tree
	trees     map[string][]mounts.File // ListAt's file lists, by repo, commit and token
	// lfsSizes maps a small blob's git id to its LFS object's size, or -1 when it isn't a
	// pointer, so ListAt reads each small blob in an LFS repo once.
	lfsSizes map[string]int64
	viewMu   sync.Mutex
}

// Option configures a Driver.
type Option func(*Driver)

// WithBaseURL points the driver at another API root, such as a GitHub Enterprise host
// ("https://ghe.example.com/api/v3") or a test's fake.
func WithBaseURL(u string) Option { return func(d *Driver) { d.base = strings.TrimRight(u, "/") } }

// WithHTTPClient sets the client for every request. The default is http.DefaultClient.
func WithHTTPClient(c *http.Client) Option { return func(d *Driver) { d.client = c } }

// WithLimits caps a snapshot's size and file count. A zero field keeps mounts.DefaultLimits'
// value. Past either, Prepare returns ErrTooLarge and stores no manifest.
func WithLimits(l mounts.Limits) Option { return func(d *Driver) { d.limits = l } }

// WithViews sets the folder where HostPath builds each snapshot's view, as hard links to its
// blobs, at <dir>/<mount name>/<commit>. It needs a blob store that is a blobs.Linker. Without
// it, HostPath returns ErrNotLocal, and the snapshot is served only as an fs.FS.
func WithViews(dir string) Option { return func(d *Driver) { d.views = dir } }

// New returns a GitHub driver keeping snapshots in store. A nil store serves live mounts only,
// and Prepare refuses a snapshot with ErrInvalidArgument.
func New(store blobs.Store, opts ...Option) *Driver {
	d := &Driver{blobs: store, base: DefaultBaseURL, client: http.DefaultClient, manifests: map[blobs.Hash]*tree{}, trees: map[string][]mounts.File{}, lfsSizes: map[string]int64{}}
	for _, o := range opts {
		o(d)
	}
	if d.limits.MaxBytes <= 0 {
		d.limits.MaxBytes = mounts.DefaultLimits.MaxBytes
	}
	if d.limits.MaxFiles <= 0 {
		d.limits.MaxFiles = mounts.DefaultLimits.MaxFiles
	}
	return d
}

// Kind returns mounts.KindGitHub.
func (d *Driver) Kind() mounts.Kind { return mounts.KindGitHub }

// Modes returns snapshot first, so a mount made with no mode is pinned.
func (d *Driver) Modes() []mounts.Mode { return []mounts.Mode{mounts.ModeSnapshot, mounts.ModeLive} }

var namePart = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)

// repo parses "github://owner/repo", with an optional ".git", into the API path "/repos/owner/repo".
func repo(source string) (string, error) {
	rest, ok := strings.CutPrefix(source, "github://")
	owner, name, two := strings.Cut(rest, "/")
	name = strings.TrimSuffix(name, ".git")
	if !ok || !two || !namePart.MatchString(owner) || !namePart.MatchString(name) || name == "." || name == ".." {
		return "", fmt.Errorf("%w: %q isn't github://owner/repo", mounts.ErrInvalidArgument, source)
	}
	return "/repos/" + owner + "/" + name, nil
}

var commitSHA = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)

// Resolve checks the repo and returns the commit ref points at. An empty ref means the repo's
// default branch. A missing repo or ref returns ErrNotFound, and so does a private repo the token
// can't read.
func (d *Driver) Resolve(ctx context.Context, source, ref string, creds mounts.Credentials) (string, error) {
	rp, err := repo(source)
	if err != nil {
		return "", err
	}
	if ref == "" {
		if ref, err = d.defaultBranch(ctx, rp, creds); err != nil {
			return "", err
		}
	}
	var c struct {
		SHA string `json:"sha"`
	}
	if err := d.call(ctx, creds, http.MethodGet, rp+"/commits/"+url.PathEscape(ref), nil, nil, false, &c); err != nil {
		return "", err
	}
	if !commitSHA.MatchString(c.SHA) {
		return "", fmt.Errorf("%w: GitHub answered commit %q for %q", mounts.ErrUnavailable, c.SHA, ref)
	}
	return c.SHA, nil
}

func (d *Driver) defaultBranch(ctx context.Context, rp string, creds mounts.Credentials) (string, error) {
	var r struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := d.call(ctx, creds, http.MethodGet, rp, nil, nil, false, &r); err != nil {
		return "", err
	}
	if r.DefaultBranch == "" {
		return "", fmt.Errorf("%w: the repo has no default branch", mounts.ErrNotFound)
	}
	return r.DefaultBranch, nil
}

// Prepare finishes a new mount after Resolve. For a snapshot it downloads m.Version's tarball
// into the blob store and sets m.Manifest. For a live mount it sets m.Ref to the default branch
// when it is empty, so later writes name the branch. A live mount's Ref must be a branch, since
// GitHub commits a write onto a branch.
func (d *Driver) Prepare(ctx context.Context, m mounts.Mount, creds mounts.Credentials) (mounts.Mount, error) {
	rp, err := repo(m.Source)
	if err != nil {
		return m, err
	}
	switch m.Mode {
	case mounts.ModeSnapshot:
		if d.blobs == nil {
			return m, fmt.Errorf("%w: this GitHub driver has no blob store for snapshots", mounts.ErrInvalidArgument)
		}
		h, err := d.snapshot(ctx, rp, m.Version, creds)
		if err != nil {
			return m, err
		}
		m.Manifest = string(h)
	case mounts.ModeLive:
		if m.Ref == "" {
			if m.Ref, err = d.defaultBranch(ctx, rp, creds); err != nil {
				return m, err
			}
		}
	default:
		return m, fmt.Errorf("%w: GitHub mounts don't support mode %q", mounts.ErrInvalidArgument, m.Mode)
	}
	return m, nil
}

// Open returns the mount's files: a snapshot's from its manifest and the blob store, or a live
// mount's from the Contents API at its branch. A live mount's fs.FS keeps ctx and creds for the
// requests it makes later.
func (d *Driver) Open(ctx context.Context, m mounts.Mount, creds mounts.Credentials) (fs.FS, error) {
	switch m.Mode {
	case mounts.ModeSnapshot:
		t, err := d.tree(ctx, m)
		if err != nil {
			return nil, err
		}
		return snapshotFS{ctx: ctx, store: d.blobs, t: t}, nil
	case mounts.ModeLive:
		rp, err := repo(m.Source)
		if err != nil {
			return nil, err
		}
		return liveFS{ctx: ctx, d: d, rp: rp, ref: m.Ref, creds: creds}, nil
	}
	return nil, fmt.Errorf("%w: GitHub mounts don't support mode %q", mounts.ErrInvalidArgument, m.Mode)
}

// call sends one API request and decodes a JSON answer into out, which may be nil. It maps
// statuses onto the mounts sentinels: 404 to ErrNotFound, 409 to ErrConflict, 422 to ErrConflict
// for a write and ErrNotFound for a read, and anything else that isn't 2xx, rate limits and
// rejected tokens included, to ErrUnavailable. An error from creds passes through unwrapped, so a
// caller can tell a connection that needs a reconnect.
func (d *Driver) call(ctx context.Context, creds mounts.Credentials, method, p string, q url.Values, body any, write bool, out any) error {
	resp, err := d.send(ctx, creds, method, p, q, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := status(resp, write); err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: decoding %s %s: %v", mounts.ErrUnavailable, method, p, err)
	}
	return nil
}

func (d *Driver) send(ctx context.Context, creds mounts.Credentials, method, p string, q url.Values, body any) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	u := d.base + p
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return d.sendURL(ctx, creds, method, u, rd, body != nil)
}

// sendURL sends one request to the full URL u, such as the GraphQL endpoint, which isn't under the
// REST base on GitHub Enterprise.
func (d *Driver) sendURL(ctx context.Context, creds mounts.Credentials, method, u string, rd io.Reader, hasBody bool) (*http.Response, error) {
	p := u
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if hasBody {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := authorize(req, creds); err != nil {
		return nil, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s %s: %v", mounts.ErrUnavailable, method, p, err)
	}
	return resp, nil
}

func status(resp *http.Response, write bool) error {
	if resp.StatusCode/100 == 2 {
		return nil
	}
	var e struct {
		Message string `json:"message"`
	}
	json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&e)
	what := fmt.Sprintf("GitHub answered %d %s", resp.StatusCode, e.Message)
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: %s", mounts.ErrNotFound, what)
	case resp.StatusCode == http.StatusConflict:
		return fmt.Errorf("%w: %s", mounts.ErrConflict, what)
	case resp.StatusCode == http.StatusUnprocessableEntity && write:
		return fmt.Errorf("%w: %s", mounts.ErrConflict, what)
	case resp.StatusCode == http.StatusUnprocessableEntity:
		return fmt.Errorf("%w: %s", mounts.ErrNotFound, what)
	}
	return fmt.Errorf("%w: %s", mounts.ErrUnavailable, what)
}

var (
	_ mounts.Preparer  = (*Driver)(nil)
	_ mounts.Writer    = (*Driver)(nil)
	_ mounts.Localizer = (*Driver)(nil)
)

// authorize sets req's bearer token from creds. Nil credentials send none. An error from the token
// source passes through unwrapped, as call documents.
func authorize(req *http.Request, creds mounts.Credentials) error {
	tok, err := mounts.BearerToken(creds)
	if err != nil {
		return err
	}
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	return nil
}
