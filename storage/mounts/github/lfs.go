package github

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"strings"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
	"golang.org/x/oauth2"
)

// lfsBatchSize is how many objects one batch request asks for. GitHub accepts 100.
const lfsBatchSize = 100

// WithLFSURL sets the web host whose "/{owner}/{repo}.git/info/lfs" serves the repo's LFS batch
// API, such as a test's fake. The default follows the API base: https://github.com for GitHub's own
// API, and the base without "/api/v3" for a GitHub Enterprise Server host.
func WithLFSURL(u string) Option { return func(d *Driver) { d.lfsURL = strings.TrimRight(u, "/") } }

func (d *Driver) lfsHost() string {
	switch {
	case d.lfsURL != "":
		return d.lfsURL
	case d.base == DefaultBaseURL:
		return "https://github.com"
	}
	return strings.TrimSuffix(d.base, "/api/v3")
}

// lfsFile is a pointer found at Path.
type lfsFile struct {
	Path string
	mounts.LFSPointer
}

// lfsAction is where one object downloads from. Header goes with the download in place of the
// mount's token, since the href is often another host.
type lfsAction struct {
	Href   string            `json:"href"`
	Header map[string]string `json:"header"`
}

// lfsErr wraps reason in mounts.ErrLFS and names the path.
func lfsErr(p string, reason error, format string, args ...any) error {
	return fmt.Errorf("%w: %w: %s: %s", mounts.ErrLFS, reason, p, fmt.Sprintf(format, args...))
}

// lfsActions asks the batch API of the repo at rp where each file's object downloads from. A
// file whose object the API refuses fails the whole call, with its path named.
func (d *Driver) lfsActions(ctx context.Context, rp string, ts oauth2.TokenSource, files []lfsFile) (map[string]lfsAction, error) {
	out := map[string]lfsAction{}
	for start := 0; start < len(files); start += lfsBatchSize {
		batch := files[start:min(start+lfsBatchSize, len(files))]
		if err := d.lfsBatch(ctx, rp, ts, batch, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (d *Driver) lfsBatch(ctx context.Context, rp string, ts oauth2.TokenSource, files []lfsFile, out map[string]lfsAction) error {
	type object struct {
		OID  string `json:"oid"`
		Size int64  `json:"size"`
	}
	req := struct {
		Operation string   `json:"operation"`
		Transfers []string `json:"transfers"`
		Objects   []object `json:"objects"`
	}{Operation: "download", Transfers: []string{"basic"}}
	byOID := map[string]string{}
	for _, f := range files {
		if _, dup := byOID[f.OID]; !dup {
			req.Objects = append(req.Objects, object{f.OID, f.Size})
			byOID[f.OID] = f.Path
		}
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	u := d.lfsHost() + "/" + strings.TrimPrefix(rp, "/repos/") + ".git/info/lfs/objects/batch"
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	hreq.Header.Set("Accept", "application/vnd.git-lfs+json")
	hreq.Header.Set("Content-Type", "application/vnd.git-lfs+json")
	if ts != nil {
		tok, err := ts.Token()
		if err != nil {
			return err
		}
		// The LFS API takes a token as a basic-auth password, as git does.
		hreq.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("x-access-token:"+tok.AccessToken)))
	}
	first := files[0].Path
	resp, err := d.client.Do(hreq)
	if err != nil {
		return lfsErr(first, mounts.ErrUnavailable, "the LFS batch API failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		var e struct {
			Message string `json:"message"`
		}
		json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&e)
		return lfsErr(first, lfsReason(resp.StatusCode), "the LFS batch API answered %d %s", resp.StatusCode, e.Message)
	}
	var got struct {
		Objects []struct {
			OID     string `json:"oid"`
			Actions struct {
				Download *lfsAction `json:"download"`
			} `json:"actions"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"objects"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		return lfsErr(first, mounts.ErrUnavailable, "decoding the LFS batch answer: %v", err)
	}
	for _, o := range got.Objects {
		p, ok := byOID[o.OID]
		if !ok {
			continue
		}
		switch {
		case o.Error != nil:
			return lfsErr(p, lfsReason(o.Error.Code), "LFS object %s: %d %s", o.OID, o.Error.Code, o.Error.Message)
		case o.Actions.Download == nil || o.Actions.Download.Href == "":
			return lfsErr(p, mounts.ErrUnavailable, "LFS object %s has no download", o.OID)
		}
		out[o.OID] = *o.Actions.Download
	}
	for oid, p := range byOID {
		if _, ok := out[oid]; !ok {
			return lfsErr(p, mounts.ErrNotFound, "the LFS batch answer leaves out object %s", oid)
		}
	}
	return nil
}

// lfsReason maps an LFS status onto a mounts sentinel. GitHub answers 403 with a message for a
// repo over its LFS bandwidth quota, so the message, not the sentinel, tells quota from access.
func lfsReason(code int) error {
	switch code {
	case http.StatusNotFound, http.StatusGone:
		return mounts.ErrNotFound
	case http.StatusUnauthorized, http.StatusForbidden:
		return mounts.ErrPermissionDenied
	}
	return mounts.ErrUnavailable
}

// lfsDownload opens f's object at act. The reader fails rather than reach EOF when the bytes
// don't match the pointer's size or oid.
func (d *Driver) lfsDownload(ctx context.Context, f lfsFile, act lfsAction) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, act.Href, nil)
	if err != nil {
		return nil, lfsErr(f.Path, mounts.ErrUnavailable, "LFS object %s: %v", f.OID, err)
	}
	for k, v := range act.Header {
		req.Header.Set(k, v)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, lfsErr(f.Path, mounts.ErrUnavailable, "downloading LFS object %s: %v", f.OID, err)
	}
	if resp.StatusCode/100 != 2 {
		resp.Body.Close()
		return nil, lfsErr(f.Path, lfsReason(resp.StatusCode), "downloading LFS object %s answered %d", f.OID, resp.StatusCode)
	}
	return &lfsReader{f: f, body: resp.Body, r: io.LimitReader(resp.Body, f.Size+1), h: sha256.New()}, nil
}

type lfsReader struct {
	f    lfsFile
	body io.Closer
	r    io.Reader
	h    hash.Hash
	n    int64
}

func (l *lfsReader) Read(b []byte) (int, error) {
	n, err := l.r.Read(b)
	l.h.Write(b[:n])
	l.n += int64(n)
	if l.n > l.f.Size {
		return n, lfsErr(l.f.Path, mounts.ErrUnavailable, "LFS object %s is over its %d bytes", l.f.OID, l.f.Size)
	}
	if err == io.EOF {
		if l.n != l.f.Size {
			return n, lfsErr(l.f.Path, mounts.ErrUnavailable, "LFS object %s is %d bytes, not %d", l.f.OID, l.n, l.f.Size)
		}
		if got := hex.EncodeToString(l.h.Sum(nil)); got != l.f.OID {
			return n, lfsErr(l.f.Path, mounts.ErrUnavailable, "LFS object %s arrived with sha256 %s", l.f.OID, got)
		}
	}
	return n, err
}

func (l *lfsReader) Close() error { return l.body.Close() }

// lfsHash is the blob store's name for f's object, which is its oid, since both are SHA-256.
func lfsHash(f lfsFile) blobs.Hash { return blobs.Hash("sha256:" + f.OID) }

// lfsStore keeps each file's object in the blob store and returns their hashes by oid. An object
// the store holds already isn't downloaded.
func (d *Driver) lfsStore(ctx context.Context, rp string, ts oauth2.TokenSource, files []lfsFile) (map[string]blobs.Hash, error) {
	out := map[string]blobs.Hash{}
	var need []lfsFile
	for _, f := range files {
		if _, seen := out[f.OID]; seen {
			continue
		}
		if has, err := d.blobs.Has(ctx, lfsHash(f)); err == nil && has {
			out[f.OID] = lfsHash(f)
			continue
		}
		if f.Size > d.limits.MaxBytes {
			return nil, fmt.Errorf("%w: %s is an LFS object of %d bytes", mounts.ErrTooLarge, f.Path, f.Size)
		}
		need = append(need, f)
		out[f.OID] = ""
	}
	if len(need) == 0 {
		return out, nil
	}
	acts, err := d.lfsActions(ctx, rp, ts, need)
	if err != nil {
		return nil, err
	}
	for _, f := range need {
		rc, err := d.lfsDownload(ctx, f, acts[f.OID])
		if err != nil {
			return nil, err
		}
		h, err := d.blobs.Put(ctx, rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		out[f.OID] = h
	}
	return out, nil
}

// lfsRead returns f's object, for a live read, which keeps nothing in the blob store.
func (d *Driver) lfsRead(ctx context.Context, rp string, ts oauth2.TokenSource, f lfsFile) ([]byte, error) {
	if f.Size > d.limits.MaxBytes {
		return nil, fmt.Errorf("%w: %s is an LFS object of %d bytes", mounts.ErrTooLarge, f.Path, f.Size)
	}
	acts, err := d.lfsActions(ctx, rp, ts, []lfsFile{f})
	if err != nil {
		return nil, err
	}
	rc, err := d.lfsDownload(ctx, f, acts[f.OID])
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}
