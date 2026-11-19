package github

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"

	"github.com/panyam/goapplib/storage/mounts"
	"golang.org/x/oauth2"
)

// content is one entry from the Contents API: a file with its contents, or one item of a folder's
// listing.
type content struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	SHA      string `json:"sha"`
	Size     int64  `json:"size"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// contents fetches p at ref. A file comes back as one entry, a folder as its listing.
func (d *Driver) contents(ctx context.Context, ts oauth2.TokenSource, rp, ref, p string) (*content, []content, error) {
	resp, err := d.send(ctx, ts, http.MethodGet, rp+"/contents/"+escapePath(p), url.Values{"ref": {ref}}, nil)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if err := status(resp, false); err != nil {
		return nil, nil, err
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", mounts.ErrUnavailable, err)
	}
	if bytes.HasPrefix(bytes.TrimSpace(b), []byte("[")) {
		var list []content
		if err := json.Unmarshal(b, &list); err != nil {
			return nil, nil, fmt.Errorf("%w: %v", mounts.ErrUnavailable, err)
		}
		return nil, list, nil
	}
	var c content
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, nil, fmt.Errorf("%w: %v", mounts.ErrUnavailable, err)
	}
	return &c, nil, nil
}

// raw fetches a file's bytes, for a file over the Contents API's 1 MB inline limit.
func (d *Driver) raw(ctx context.Context, ts oauth2.TokenSource, rp, ref, p string) ([]byte, error) {
	u := d.base + rp + "/contents/" + escapePath(p) + "?" + url.Values{"ref": {ref}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github.raw+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if ts != nil {
		tok, err := ts.Token()
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", mounts.ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if err := status(resp, false); err != nil {
		return nil, err
	}
	return io.ReadAll(resp.Body)
}

func escapePath(p string) string {
	return (&url.URL{Path: p}).EscapedPath()
}

// liveFS reads a branch through the Contents API on every open, so it shows the branch's latest
// commit. Links and submodules don't appear in listings, and opening one fails with
// mounts.ErrInvalidPath.
type liveFS struct {
	ctx context.Context
	d   *Driver
	rp  string
	ref string
	ts  oauth2.TokenSource
}

func (l liveFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, pathErr("open", name, fs.ErrInvalid)
	}
	p := name
	if p == "." {
		p = ""
	}
	c, list, err := l.d.contents(l.ctx, l.ts, l.rp, l.ref, p)
	if errors.Is(err, mounts.ErrNotFound) {
		return nil, pathErr("open", name, fs.ErrNotExist)
	}
	if err != nil {
		return nil, pathErr("open", name, err)
	}
	if c == nil {
		var ents []fs.DirEntry
		for _, e := range list {
			switch e.Type {
			case "file":
				ents = append(ents, info{name: e.Name, size: e.Size})
			case "dir":
				ents = append(ents, info{name: e.Name, dir: true})
			}
		}
		return newDir(name, ents), nil
	}
	if c.Type != "file" {
		return nil, pathErr("open", name, mounts.ErrInvalidPath)
	}
	data, err := base64.StdEncoding.DecodeString(c.Content)
	if err != nil || (c.Encoding != "base64" || int64(len(data)) != c.Size) && c.Size > 0 {
		if data, err = l.d.raw(l.ctx, l.ts, l.rp, l.ref, p); err != nil {
			return nil, pathErr("open", name, err)
		}
	}
	return &file{info: info{name: path.Base(name), size: int64(len(data))}, rc: io.NopCloser(bytes.NewReader(data))}, nil
}

func (d *Driver) live(m mounts.Mount, p string) (string, string, error) {
	if m.Mode != mounts.ModeLive {
		return "", "", fmt.Errorf("%w: GitHub mount %q is a snapshot", mounts.ErrReadOnly, m.Name)
	}
	rp, err := repo(m.Source)
	if err != nil {
		return "", "", err
	}
	clean, err := mounts.CleanPath(p)
	if err != nil {
		return "", "", err
	}
	if clean == "" {
		return "", "", fmt.Errorf("%w: a write needs a file path", mounts.ErrInvalidPath)
	}
	return rp, clean, nil
}

// Stat returns the file's git blob sha, the version a Put or Delete passes as IfMatch. A folder
// returns ErrInvalidArgument, and a missing file ErrNotFound. A snapshot mount returns
// ErrReadOnly, since its files have no versions to write against.
func (d *Driver) Stat(ctx context.Context, m mounts.Mount, ts oauth2.TokenSource, p string) (mounts.FileVersion, error) {
	rp, clean, err := d.live(m, p)
	if err != nil {
		return "", err
	}
	c, _, err := d.contents(ctx, ts, rp, m.Ref, clean)
	if err != nil {
		return "", err
	}
	if c == nil || c.Type != "file" {
		return "", fmt.Errorf("%w: %q isn't a file", mounts.ErrInvalidArgument, clean)
	}
	return mounts.FileVersion(c.SHA), nil
}

type writeResp struct {
	Content *struct {
		SHA string `json:"sha"`
	} `json:"content"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

// ifMatch turns a request's IfMatch into the sha GitHub wants: none to create, the file's
// current sha for AnyVersion, or the caller's.
func (d *Driver) ifMatch(ctx context.Context, m mounts.Mount, ts oauth2.TokenSource, p string, v mounts.FileVersion) (string, error) {
	if v != mounts.AnyVersion {
		return string(v), nil
	}
	cur, err := d.Stat(ctx, m, ts, p)
	if errors.Is(err, mounts.ErrNotFound) {
		return "", nil
	}
	return string(cur), err
}

// Put writes req.Data to req.Path as one commit on the mount's branch. An empty IfMatch sends no
// sha, so GitHub refuses to replace an existing file and Put returns ErrConflict. A stale IfMatch
// returns ErrConflict too. The result names the new commit and the file's new blob sha.
func (d *Driver) Put(ctx context.Context, m mounts.Mount, ts oauth2.TokenSource, req mounts.PutRequest) (*mounts.WriteResult, error) {
	rp, p, err := d.live(m, req.Path)
	if err != nil {
		return nil, err
	}
	sha, err := d.ifMatch(ctx, m, ts, p, req.IfMatch)
	if err != nil {
		return nil, err
	}
	msg := req.Message
	if msg == "" {
		msg = "Update " + p
	}
	body := map[string]string{"message": msg, "content": base64.StdEncoding.EncodeToString(req.Data), "branch": m.Ref}
	if sha != "" {
		body["sha"] = sha
	}
	var out writeResp
	if err := d.call(ctx, ts, http.MethodPut, rp+"/contents/"+escapePath(p), nil, body, true, &out); err != nil {
		return nil, err
	}
	res := &mounts.WriteResult{Mount: out.Commit.SHA}
	if out.Content != nil {
		res.File = mounts.FileVersion(out.Content.SHA)
	}
	return res, nil
}

// Delete removes req.Path as one commit on the mount's branch. IfMatch must be the file's current
// version or AnyVersion; a stale one returns ErrConflict, and an empty one ErrInvalidArgument.
func (d *Driver) Delete(ctx context.Context, m mounts.Mount, ts oauth2.TokenSource, req mounts.DeleteRequest) (*mounts.WriteResult, error) {
	rp, p, err := d.live(m, req.Path)
	if err != nil {
		return nil, err
	}
	if req.IfMatch == "" {
		return nil, fmt.Errorf("%w: Delete needs IfMatch, the file's version or AnyVersion", mounts.ErrInvalidArgument)
	}
	sha, err := d.ifMatch(ctx, m, ts, p, req.IfMatch)
	if err != nil {
		return nil, err
	}
	if sha == "" {
		return nil, fmt.Errorf("%w: %q", mounts.ErrNotFound, p)
	}
	msg := req.Message
	if msg == "" {
		msg = "Delete " + p
	}
	var out writeResp
	body := map[string]string{"message": msg, "sha": sha, "branch": m.Ref}
	if err := d.call(ctx, ts, http.MethodDelete, rp+"/contents/"+escapePath(p), nil, body, true, &out); err != nil {
		return nil, err
	}
	return &mounts.WriteResult{Mount: out.Commit.SHA}, nil
}
