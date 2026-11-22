// Package msgraph serves mounts over folders in SharePoint document libraries and OneDrives,
// through Microsoft Graph's REST API. A source names a drive and a folder in it:
//
//	msgraph://sites/<site-id>/drives/<drive-id>/root/<folder>
//	msgraph://drives/<drive-id>/root/<folder>
//	msgraph://me/drive/root/<folder>
//
// It is its own module, like the S3 and GCS drivers, though it uses plain HTTP and no Graph SDK.
// The listing, pinning and fs.FS logic is in storage/mounts/bucket, and this package adapts a
// drive to its Bucket interface, with the part of the source before the first "/" as the bucket
// name.
//
// A drive has no change that spans several files, so a snapshot's version is the hash of a pinned
// listing, as for a bucket. Each file is pinned by its item id, its content tag (cTag, or eTag
// where Graph sends no cTag), its size and its last-modified time. A pinned read serves the item
// while its content tag still matches. After a change it serves the entry in the item's version
// history with the pinned modified time and size. When the library keeps no such version, the read
// fails with the path named and mounts.ErrConflict.
//
// Delta asks Graph what changed since a token, so a poll reads only the changes. Subscriptions
// are the push form: CreateSubscription and RenewSubscription manage one, AnswerValidation answers
// Graph's validation handshake, and ParseNotification turns a notification into a mounts.Change.
//
// The driver signs with mounts.BearerCredentials, which a connections KindEntra or KindEntraUser
// connection gives. Any other kind returns mounts.ErrInvalidArgument.
package msgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/bucket"
	"golang.org/x/oauth2"
)

// DefaultBaseURL is Microsoft Graph's v1.0 endpoint for public Azure. WithBaseURL sets another,
// for a national cloud or a test's fake.
const DefaultBaseURL = "https://graph.microsoft.com/v1.0"

// Scheme is the scheme of a msgraph source.
const Scheme = "msgraph"

// ErrResyncRequired means Graph no longer knows a delta token, so the app has to read the mount
// again in full and start over with an empty token. Graph answers 410 Gone for it.
var ErrResyncRequired = errors.New("msgraph: delta token expired, resync required")

// Driver serves mounts.KindMSGraph mounts. The embedded bucket.Driver implements mounts.Driver,
// mounts.Preparer and mounts.VersionReader. Driver adds Delta and the subscription calls.
type Driver struct {
	*bucket.Driver
	base   string
	client *http.Client
}

type options struct {
	base   string
	client *http.Client
	limits mounts.Limits
}

// Option configures a Driver.
type Option func(*options)

// WithBaseURL points the driver at another Graph root than DefaultBaseURL, such as a national
// cloud's ("https://graph.microsoft.us/v1.0") or a test's fake.
func WithBaseURL(u string) Option { return func(o *options) { o.base = strings.TrimRight(u, "/") } }

// WithHTTPClient sets the client for every request. The default is http.DefaultClient.
func WithHTTPClient(c *http.Client) Option { return func(o *options) { o.client = c } }

// WithLimits caps how many files and bytes a source may hold. A zero field keeps
// mounts.DefaultLimits' value.
func WithLimits(l mounts.Limits) Option { return func(o *options) { o.limits = l } }

// New returns a Graph driver keeping snapshot listings in store.
func New(store blobs.Store, opts ...Option) *Driver {
	o := options{base: DefaultBaseURL, client: http.DefaultClient}
	for _, f := range opts {
		f(&o)
	}
	d := &Driver{base: o.base, client: o.client}
	d.Driver = bucket.New(mounts.KindMSGraph, Scheme, d.connect, store, bucket.WithLimits(o.limits))
	return d
}

// connect opens the "bucket" name, which is the first segment of a source: sites, drives or me.
func (d *Driver) connect(ctx context.Context, name string, creds mounts.Credentials) (bucket.Bucket, error) {
	c, err := d.graph(creds)
	if err != nil {
		return nil, err
	}
	switch name {
	case "sites", "drives", "me":
	default:
		return nil, fmt.Errorf("%w: a msgraph source starts with sites/, drives/ or me/, not %q", mounts.ErrInvalidArgument, name)
	}
	return &drive{g: c, kind: name}, nil
}

func (d *Driver) graph(creds mounts.Credentials) (*graph, error) {
	ts, err := mounts.TokenSourceOf(creds)
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(d.base)
	if err != nil {
		return nil, fmt.Errorf("%w: base URL: %v", mounts.ErrInvalidArgument, err)
	}
	return &graph{base: d.base, origin: base.Scheme + "://" + base.Host, client: d.client, ts: ts}, nil
}

// graph sends requests to one Graph root with one token source.
type graph struct {
	base   string
	origin string
	client *http.Client
	ts     oauth2.TokenSource
}

// url turns a path under the Graph root into a URL. A full URL, such as an @odata.nextLink, must
// be on the Graph root's host, so a link in an answer can't send the token elsewhere.
func (g *graph) url(p string) (string, error) {
	if strings.HasPrefix(p, "/") {
		return g.base + p, nil
	}
	if !strings.HasPrefix(p, g.origin+"/") {
		return "", fmt.Errorf("%w: Graph sent a link off its own host: %q", mounts.ErrUnavailable, p)
	}
	return p, nil
}

func (g *graph) do(ctx context.Context, method, p string, body any) (*http.Response, error) {
	u, err := g.url(p)
	if err != nil {
		return nil, err
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if g.ts != nil {
		tok, err := g.ts.Token()
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s %s: %v", mounts.ErrUnavailable, method, p, err)
	}
	return resp, nil
}

// call sends one request and decodes a JSON answer into out, which may be nil.
func (g *graph) call(ctx context.Context, method, p string, body, out any) error {
	resp, err := g.do(ctx, method, p, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := status(resp, p); err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: decoding %s: %v", mounts.ErrUnavailable, p, err)
	}
	return nil
}

// status maps a Graph answer onto the mounts sentinels: 404 to ErrNotFound, 401 and 403 to
// ErrPermissionDenied, 410 to ErrResyncRequired, and anything else that isn't 2xx, throttling
// included, to ErrUnavailable. The message carries Graph's error code.
func status(resp *http.Response, p string) error {
	if resp.StatusCode/100 == 2 {
		return nil
	}
	var e struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&e)
	what := fmt.Sprintf("Graph answered %d %s for %s", resp.StatusCode, e.Error.Code, p)
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		what += ", retry after " + ra + "s"
	}
	switch resp.StatusCode {
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", mounts.ErrNotFound, what)
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: %s", mounts.ErrPermissionDenied, what)
	case http.StatusGone:
		return fmt.Errorf("%w: %s", ErrResyncRequired, what)
	}
	return fmt.Errorf("%w: %s", mounts.ErrUnavailable, what)
}

// escapePath escapes each segment of a drive path, so a name with ":" or "?" can't end the
// path-based address early.
func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

var _ mounts.VersionReader = (*Driver)(nil)
