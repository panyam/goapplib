package msgraph

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/bucket"
)

// pageSize is how many children one listing request asks for.
const pageSize = 200

// itemSelect is the driveItem fields a listing reads.
const itemSelect = "id,name,eTag,cTag,size,lastModifiedDateTime,file,folder,deleted,parentReference"

// item is the part of a driveItem the driver reads.
type item struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ETag         string    `json:"eTag"`
	CTag         string    `json:"cTag"`
	Size         int64     `json:"size"`
	LastModified string    `json:"lastModifiedDateTime"`
	File         *struct{} `json:"file"`
	Folder       *struct{} `json:"folder"`
	Deleted      *struct{} `json:"deleted"`
	Parent       *struct {
		Path string `json:"path"`
	} `json:"parentReference"`
}

// contentTag is what a pin compares to tell whether a file's bytes changed. Graph changes cTag
// only when the content does, and eTag on any change, a rename included, so cTag is preferred.
func (it item) contentTag() string {
	if it.CTag != "" {
		return it.CTag
	}
	return it.ETag
}

// drive is a bucket.Bucket over one source's first segment. Its keys are the source's path after
// that segment: "<site>/drives/<drive>/root/<path>" for sites, "<drive>/root/<path>" for drives,
// and "drive/root/<path>" for me.
type drive struct {
	g    *graph
	kind string
}

// loc is a key split into the drive's API path, such as "/drives/b!x" with each segment escaped,
// the same path unescaped, which a subscription names as its resource, the key's head up to and
// including "root", and the path inside the drive's root.
type loc struct {
	api      string
	resource string
	head     string
	path     string
}

// split parses key, which may end in "/".
func (d *drive) split(key string) (loc, error) {
	segs := strings.Split(strings.TrimSuffix(key, "/"), "/")
	var rootAt int
	switch d.kind {
	case "sites":
		if len(segs) < 4 || segs[1] != "drives" {
			return loc{}, fmt.Errorf("%w: msgraph://sites/%s doesn't name sites/<site>/drives/<drive>/root", mounts.ErrInvalidArgument, key)
		}
		rootAt = 3
	case "drives":
		rootAt = 1
	case "me":
		if len(segs) < 2 || segs[0] != "drive" {
			return loc{}, fmt.Errorf("%w: msgraph://me/%s doesn't name me/drive/root", mounts.ErrInvalidArgument, key)
		}
		rootAt = 1
	}
	if len(segs) <= rootAt || segs[rootAt] != "root" {
		return loc{}, fmt.Errorf("%w: msgraph://%s/%s has no root/ after its drive", mounts.ErrInvalidArgument, d.kind, key)
	}
	api := "/" + d.kind
	for _, s := range segs[:rootAt] {
		api += "/" + url.PathEscape(s)
	}
	resource := strings.Join(append([]string{d.kind}, segs[:rootAt]...), "/")
	return loc{api: api, resource: resource, head: strings.Join(segs[:rootAt+1], "/"), path: strings.Join(segs[rootAt+1:], "/")}, nil
}

func (l loc) key(p string) string {
	if p == "" {
		return l.head
	}
	return l.head + "/" + p
}

// childrenURL addresses the children of the folder at p, "" being the drive's root.
func (l loc) childrenURL(p string) string {
	q := "?$top=" + fmt.Sprint(pageSize) + "&$select=" + itemSelect
	if p == "" {
		return l.api + "/root/children" + q
	}
	return l.api + "/root:/" + escapePath(p) + ":/children" + q
}

// children lists the folder at p, following @odata.nextLink.
func (d *drive) children(ctx context.Context, l loc, p string) ([]item, error) {
	var out []item
	next := l.childrenURL(p)
	for next != "" {
		var page struct {
			Value    []item `json:"value"`
			NextLink string `json:"@odata.nextLink"`
		}
		if err := d.g.call(ctx, http.MethodGet, next, nil, &page); err != nil {
			return nil, err
		}
		out = append(out, page.Value...)
		next = page.NextLink
	}
	return out, nil
}

func object(l loc, p string, it item) bucket.Object {
	return bucket.Object{Key: l.key(p), Version: it.ID + "@" + it.LastModified, ETag: it.contentTag(), Size: it.Size}
}

// List walks the folder prefix names, a folder at a time, and returns every file under it.
func (d *drive) List(ctx context.Context, prefix string) ([]bucket.Object, error) {
	l, err := d.split(prefix)
	if err != nil {
		return nil, err
	}
	var out []bucket.Object
	folders := []string{l.path}
	for len(folders) > 0 {
		dir := folders[0]
		folders = folders[1:]
		kids, err := d.children(ctx, l, dir)
		if err != nil {
			return nil, err
		}
		for _, it := range kids {
			p := it.Name
			if dir != "" {
				p = dir + "/" + it.Name
			}
			switch {
			case it.Folder != nil:
				folders = append(folders, p)
			case it.File != nil:
				out = append(out, object(l, p, it))
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// ListDir lists the files and folders directly in dir.
func (d *drive) ListDir(ctx context.Context, dir string) ([]bucket.Object, []string, error) {
	l, err := d.split(dir)
	if err != nil {
		return nil, nil, err
	}
	kids, err := d.children(ctx, l, l.path)
	if err != nil {
		return nil, nil, err
	}
	var files []bucket.Object
	var folders []string
	for _, it := range kids {
		p := it.Name
		if l.path != "" {
			p = l.path + "/" + it.Name
		}
		switch {
		case it.Folder != nil:
			folders = append(folders, it.Name)
		case it.File != nil:
			files = append(files, object(l, p, it))
		}
	}
	return files, folders, nil
}

// Get reads a file. Unpinned, it reads the file at key now. Pinned, it reads the item the pin
// names while its content tag matches, then the version-history entry with the pinned modified
// time and size.
func (d *drive) Get(ctx context.Context, key string, pin *bucket.Object) (io.ReadCloser, int64, error) {
	l, err := d.split(key)
	if err != nil {
		return nil, 0, err
	}
	if pin == nil {
		var it item
		if err := d.g.call(ctx, http.MethodGet, l.api+"/root:/"+escapePath(l.path), nil, &it); err != nil {
			return nil, 0, err
		}
		if it.File == nil {
			return nil, 0, fmt.Errorf("%w: %s isn't a file", mounts.ErrNotFound, l.path)
		}
		return d.content(ctx, l.api+"/items/"+url.PathEscape(it.ID)+"/content", it.Size)
	}
	id, modified, ok := strings.Cut(pin.Version, "@")
	if !ok || id == "" {
		return nil, 0, fmt.Errorf("%w: pin %q for %s isn't an item id and a time", mounts.ErrInvalidArgument, pin.Version, l.path)
	}
	itemURL := l.api + "/items/" + url.PathEscape(id)
	var it item
	err = d.g.call(ctx, http.MethodGet, itemURL+"?$select="+itemSelect, nil, &it)
	switch {
	case err == nil && it.contentTag() == pin.ETag:
		return d.content(ctx, itemURL+"/content", it.Size)
	case err != nil && !isNotFound(err):
		return nil, 0, err
	}
	var vs struct {
		Value []struct {
			ID           string `json:"id"`
			LastModified string `json:"lastModifiedDateTime"`
			Size         int64  `json:"size"`
		} `json:"value"`
	}
	if err == nil {
		if err := d.g.call(ctx, http.MethodGet, itemURL+"/versions", nil, &vs); err != nil && !isNotFound(err) {
			return nil, 0, err
		}
	}
	for _, v := range vs.Value {
		if v.Size == pin.Size && sameTime(v.LastModified, modified) {
			return d.content(ctx, itemURL+"/versions/"+url.PathEscape(v.ID)+"/content", v.Size)
		}
	}
	return nil, 0, fmt.Errorf("%w: %s changed since the snapshot listed it, and the library keeps no version from %s", mounts.ErrConflict, l.path, modified)
}

func (d *drive) content(ctx context.Context, p string, size int64) (io.ReadCloser, int64, error) {
	resp, err := d.g.do(ctx, http.MethodGet, p, nil)
	if err != nil {
		return nil, 0, err
	}
	if err := status(resp, p); err != nil {
		resp.Body.Close()
		return nil, 0, err
	}
	return resp.Body, size, nil
}

func sameTime(a, b string) bool {
	ta, errA := time.Parse(time.RFC3339Nano, a)
	tb, errB := time.Parse(time.RFC3339Nano, b)
	if errA != nil || errB != nil {
		return a == b
	}
	return ta.Equal(tb)
}

func isNotFound(err error) bool { return errors.Is(err, mounts.ErrNotFound) }
