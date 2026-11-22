package msgraph

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/bucket"
)

// DeltaItem is one file that changed under a mount since a delta token.
type DeltaItem struct {
	// Path is the file's path inside the mount. It is empty for a deleted item whose path Graph
	// didn't send, which SharePoint and OneDrive for Business do for deletions.
	Path string

	// ItemID is the item's Graph id, which names the file even when Path is empty.
	ItemID string

	// Deleted is true when the file was deleted.
	Deleted bool
}

// DeltaPage is what one Delta call found.
type DeltaPage struct {
	// Items are the files under the mount that changed, in the order Graph sent them. Folders
	// are left out.
	Items []DeltaItem

	// Token is what the next Delta call passes. The app keeps it beside the mount.
	Token string
}

// Delta returns the files under m's folder that changed since token, and the token for the next
// call. An empty token returns no items and a token for "now", so an app starts following a mount
// from the moment it is made. Graph tracks changes for a whole drive, so Delta asks for the
// drive's changes and keeps those under m's folder, along with deletions whose path Graph left
// out. A token Graph has forgotten returns ErrResyncRequired; the app then reads the mount in full
// and starts over with an empty token.
func (d *Driver) Delta(ctx context.Context, m mounts.Mount, creds mounts.Credentials, token string) (DeltaPage, error) {
	dr, l, err := d.locate(m.Source, creds)
	if err != nil {
		return DeltaPage{}, err
	}
	if token == "" {
		token = "latest"
	}
	next := l.api + "/root/delta?token=" + url.QueryEscape(token) + "&$select=" + itemSelect
	var out DeltaPage
	for next != "" {
		var page struct {
			Value     []item `json:"value"`
			NextLink  string `json:"@odata.nextLink"`
			DeltaLink string `json:"@odata.deltaLink"`
		}
		if err := dr.g.call(ctx, http.MethodGet, next, nil, &page); err != nil {
			return DeltaPage{}, err
		}
		for _, it := range page.Value {
			if it.Folder != nil {
				continue
			}
			p, known := drivePath(it)
			if !known {
				if it.Deleted != nil {
					out.Items = append(out.Items, DeltaItem{ItemID: it.ID, Deleted: true})
				}
				continue
			}
			rel, ok := under(p, l.path)
			if !ok {
				continue
			}
			out.Items = append(out.Items, DeltaItem{Path: rel, ItemID: it.ID, Deleted: it.Deleted != nil})
		}
		next = page.NextLink
		if page.DeltaLink != "" {
			u, err := url.Parse(page.DeltaLink)
			if err != nil || u.Query().Get("token") == "" {
				return DeltaPage{}, fmt.Errorf("%w: Graph's delta link %q carries no token", mounts.ErrUnavailable, page.DeltaLink)
			}
			out.Token = u.Query().Get("token")
		}
	}
	return out, nil
}

// locate parses source and opens its drive.
func (d *Driver) locate(source string, creds mounts.Credentials) (*drive, loc, error) {
	src, err := bucket.ParseSource(Scheme, source)
	if err != nil {
		return nil, loc{}, err
	}
	b, err := d.connect(context.Background(), src.Bucket, creds)
	if err != nil {
		return nil, loc{}, err
	}
	dr := b.(*drive)
	l, err := dr.split(src.Prefix)
	return dr, l, err
}

// drivePath is an item's path from the drive's root, from its parentReference.path, such as
// "/drives/b!x/root:/Boards". It reports false when Graph sent no path.
func drivePath(it item) (string, bool) {
	if it.Parent == nil || it.Parent.Path == "" || it.Name == "" {
		return "", false
	}
	_, dir, ok := strings.Cut(it.Parent.Path, "root:")
	if !ok {
		return "", false
	}
	dir, err := url.PathUnescape(strings.Trim(dir, "/"))
	if err != nil {
		return "", false
	}
	if dir == "" {
		return it.Name, true
	}
	return dir + "/" + it.Name, true
}

// under returns p relative to folder, or false when p isn't inside it.
func under(p, folder string) (string, bool) {
	if folder == "" {
		return p, true
	}
	rel, ok := strings.CutPrefix(p, folder+"/")
	return rel, ok && rel != ""
}
