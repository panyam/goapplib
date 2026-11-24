package github

import (
	"bytes"
	"context"
	"crypto/sha1" // #nosec G505 -- git names a blob by SHA-1, and this checks a download against that name
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
)

// DefaultRawURL is the host that serves a repo's files by commit and path. Its downloads don't
// count against the REST API's hourly limit, which is 60 requests for a caller with no token.
const DefaultRawURL = "https://raw.githubusercontent.com"

// gitAliases is the namespace a git blob id is recorded under in a blobs.Aliases store.
const gitAliases = "git"

// treeCacheSize caps how many commits' file lists a Driver keeps for ListAt.
const treeCacheSize = 16

// WithRawURL sets the host FetchAt downloads files from, by commit and path, such as a test's
// fake. The default is DefaultRawURL when the API base is GitHub's own. With another API base, such
// as GitHub Enterprise, and no raw URL, FetchAt uses the git blobs API instead.
func WithRawURL(u string) Option { return func(d *Driver) { d.rawURL = strings.TrimRight(u, "/") } }

var gitBlobID = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Moves reports whether ref moves: an empty ref, which is the default branch, or a branch. A full
// commit sha doesn't, and neither does a tag.
func (d *Driver) Moves(ctx context.Context, source, ref string, creds mounts.Credentials) (bool, error) {
	rp, err := repo(source)
	if err != nil {
		return false, err
	}
	if ref == "" {
		return true, nil
	}
	if commitSHA.MatchString(ref) {
		return false, nil
	}
	err = d.call(ctx, creds, http.MethodGet, rp+"/branches/"+url.PathEscape(ref), nil, nil, false, nil)
	if errors.Is(err, mounts.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// ListAt lists the files at commit under the path under, from one call to the trees API per commit.
// A tree GitHub truncates returns ErrTooLarge, since a partial list would hide files.
func (d *Driver) ListAt(ctx context.Context, m mounts.Mount, creds mounts.Credentials, commit, under string) ([]mounts.File, error) {
	all, err := d.treeAt(ctx, m, creds, commit)
	if err != nil {
		return nil, err
	}
	out := (&lazyTree{all: all}).under(under)
	if err := d.within(out, under, commit); err != nil {
		return nil, err
	}
	return out, nil
}

func shown(p string) string {
	if p == "" {
		return "the repo"
	}
	return p
}

// treeKey names one listing of rp at commit. It carries a hash of the token, so a list read with
// one connection's token is never served to a caller with another token or none, which would show
// a private repo's files.
func treeKey(rp, commit string, creds mounts.Credentials) (string, error) {
	key := rp + "@" + commit
	tok, err := mounts.BearerToken(creds)
	if err != nil {
		return "", err
	}
	if tok != "" {
		sum := sha256.Sum256([]byte(tok))
		key += "#" + hex.EncodeToString(sum[:8])
	}
	return key, nil
}

// treeAt is every regular file in the repo at commit, sorted by path. Links (mode 120000),
// submodules and paths that are unclean or climb out are left out.
func (d *Driver) treeAt(ctx context.Context, m mounts.Mount, creds mounts.Credentials, commit string) ([]mounts.File, error) {
	rp, err := repo(m.Source)
	if err != nil {
		return nil, err
	}
	if !commitSHA.MatchString(commit) {
		return nil, fmt.Errorf("%w: version %q isn't a commit sha", mounts.ErrInvalidArgument, commit)
	}
	key, err := treeKey(rp, commit, creds)
	if err != nil {
		return nil, err
	}
	d.mu.Lock()
	cached, ok := d.trees[key]
	d.mu.Unlock()
	if ok {
		return cached, nil
	}
	var t struct {
		Tree []struct {
			Path string `json:"path"`
			Mode string `json:"mode"`
			Type string `json:"type"`
			SHA  string `json:"sha"`
			Size int64  `json:"size"`
		} `json:"tree"`
		Truncated bool `json:"truncated"`
	}
	if err := d.call(ctx, creds, http.MethodGet, rp+"/git/trees/"+commit, url.Values{"recursive": {"1"}}, nil, false, &t); err != nil {
		return nil, err
	}
	if t.Truncated {
		return nil, fmt.Errorf("%w: GitHub lists the tree at %s only in part", mounts.ErrTooLarge, commit)
	}
	var out []mounts.File
	for _, e := range t.Tree {
		if e.Type != "blob" || e.Mode == "120000" || !gitBlobID.MatchString(e.SHA) {
			continue
		}
		if p, err := mounts.CleanPath(e.Path); err != nil || p != e.Path || p == "" {
			continue
		}
		out = append(out, mounts.File{Path: e.Path, ID: e.SHA, Size: e.Size})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	if err := d.lfsSizesAt(ctx, m, creds, commit, out); err != nil {
		return nil, err
	}
	d.mu.Lock()
	if len(d.trees) >= treeCacheSize {
		clear(d.trees)
	}
	d.trees[key] = out
	d.mu.Unlock()
	return out, nil
}

// lfsSizesCap caps how many small blobs' LFS answers a Driver keeps.
const lfsSizesCap = 50_000

// A Git LFS pointer with no extension lines is 125 to 144 bytes: the version line, the oid line,
// and a size of 1 to 19 digits, with or without the last newline.
const (
	minLFSPointer = 125
	maxLFSPointer = 144
)

// lfsSizesAt sets the Size of each Git LFS pointer in files to its object's size (#208), so ListAt
// reports the size FetchAt keeps. When a .gitattributes file at commit has filter=lfs, it reads
// every blob up to mounts.MaxLFSPointerSize. Without one, it reads only blobs of a pointer's size,
// since LFS paths can be set outside the repo, in .git/info/attributes, and FetchAt swaps every
// pointer it meets (#218). So a pointer with extension lines in such a repo keeps its own size. It
// remembers each answer by git blob id, so a later commit reads only the small blobs that changed.
// Its result goes into treeAt's cache with the list, so one commit costs these reads once per token.
func (d *Driver) lfsSizesAt(ctx context.Context, m mounts.Mount, creds mounts.Credentials, commit string, files []mounts.File) error {
	lfs := false
	for _, f := range files {
		if path.Base(f.Path) != ".gitattributes" {
			continue
		}
		b, err := d.download(ctx, m, creds, commit, f)
		if err != nil {
			return err
		}
		if bytes.Contains(b, []byte("filter=lfs")) {
			lfs = true
			break
		}
	}
	for i, f := range files {
		if f.Size > mounts.MaxLFSPointerSize || !lfs && (f.Size < minLFSPointer || f.Size > maxLFSPointer) {
			continue
		}
		d.mu.Lock()
		size, known := d.lfsSizes[f.ID]
		d.mu.Unlock()
		if !known {
			b, err := d.download(ctx, m, creds, commit, f)
			if err != nil {
				return err
			}
			size = -1
			if ptr, ok := mounts.ParseLFSPointer(b); ok && gitBlobSHA(b) == f.ID {
				size = ptr.Size
			}
			d.mu.Lock()
			if len(d.lfsSizes) >= lfsSizesCap {
				clear(d.lfsSizes)
			}
			d.lfsSizes[f.ID] = size
			d.mu.Unlock()
		}
		if size >= 0 {
			files[i].Size = size
		}
	}
	return nil
}

// FetchAt keeps f's contents in the blob store. When the store is a blobs.Aliases that maps f.ID
// to a blob it holds, nothing is downloaded. A download is checked against f.ID before it's kept.
// A Git LFS pointer is swapped for its object, and f.ID then maps to the object's blob.
func (d *Driver) FetchAt(ctx context.Context, m mounts.Mount, creds mounts.Credentials, commit string, f mounts.File) (blobs.Hash, bool, error) {
	if d.blobs == nil {
		return "", false, fmt.Errorf("%w: this GitHub driver has no blob store", mounts.ErrInvalidArgument)
	}
	if !gitBlobID.MatchString(f.ID) {
		return "", false, fmt.Errorf("%w: %s has no git blob id", mounts.ErrInvalidArgument, f.Path)
	}
	al, _ := d.blobs.(blobs.Aliases)
	if al != nil {
		if h, ok, err := al.Alias(ctx, gitAliases, f.ID); err == nil && ok {
			if has, err := d.blobs.Has(ctx, h); err == nil && has {
				return h, false, nil
			}
		}
	}
	if f.Size > d.limits.MaxBytes {
		return "", false, fmt.Errorf("%w: %s is %d bytes", mounts.ErrTooLarge, f.Path, f.Size)
	}
	b, err := d.download(ctx, m, creds, commit, f)
	if err != nil {
		return "", false, err
	}
	if id := gitBlobSHA(b); id != f.ID {
		return "", false, fmt.Errorf("%w: GitHub sent %s with git blob id %s, not %s", mounts.ErrUnavailable, f.Path, id, f.ID)
	}
	var h blobs.Hash
	if ptr, ok := mounts.ParseLFSPointer(b); ok {
		rp, err := repo(m.Source)
		if err != nil {
			return "", false, err
		}
		hashes, err := d.lfsStore(ctx, rp, creds, []lfsFile{{Path: f.Path, LFSPointer: ptr}})
		if err != nil {
			return "", false, err
		}
		h = hashes[ptr.OID]
	} else if h, err = d.blobs.Put(ctx, bytes.NewReader(b)); err != nil {
		return "", false, err
	}
	if al != nil {
		if err := al.SetAlias(ctx, gitAliases, f.ID, h); err != nil {
			return "", false, err
		}
	}
	return h, true, nil
}

// download reads f at commit from the raw host, or from the git blobs API when the driver has no
// raw host.
func (d *Driver) download(ctx context.Context, m mounts.Mount, creds mounts.Credentials, commit string, f mounts.File) ([]byte, error) {
	rp, err := repo(m.Source)
	if err != nil {
		return nil, err
	}
	raw := d.rawURL
	if raw == "" && d.base == DefaultBaseURL {
		raw = DefaultRawURL
	}
	u := d.base + rp + "/git/blobs/" + f.ID
	accept := "application/vnd.github.raw+json"
	if raw != "" {
		u = raw + "/" + strings.TrimPrefix(rp, "/repos/") + "/" + commit + "/" + escapePath(f.Path)
		accept = "*/*"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	if raw == "" {
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}
	if err := authorize(req, creds); err != nil {
		return nil, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", mounts.ErrUnavailable, f.Path, err)
	}
	defer resp.Body.Close()
	if err := status(resp, false); err != nil {
		return nil, err
	}
	// f.Size is an LFS object's size when ListAt found a pointer, and the object can be smaller
	// than the pointer (#218). The blob id check after this catches anything cut short.
	b, err := io.ReadAll(io.LimitReader(resp.Body, max(f.Size, mounts.MaxLFSPointerSize)+1))
	if err != nil {
		return nil, fmt.Errorf("%w: reading %s: %v", mounts.ErrUnavailable, f.Path, err)
	}
	return b, nil
}

// gitBlobSHA is the id git gives contents b.
func gitBlobSHA(b []byte) string {
	h := sha1.New() // #nosec G401 -- git's own object id, checked against GitHub's file list
	fmt.Fprintf(h, "blob %d\x00", len(b))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil))
}

var _ mounts.VersionReader = (*Driver)(nil)
