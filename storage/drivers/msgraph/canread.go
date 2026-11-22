package msgraph

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/panyam/goapplib/storage/mounts"
)

// CanRead reports whether creds can read the folder at source, such as
// "msgraph://sites/<site>/drives/<drive>/root/Boards". It fits connections.ReadCheck, so an app
// passes it to connections.WithReadCheckFor("msgraph", d.CanRead, connections.KindEntraUser), and
// a person's own Entra sign-in decides whether they see a SharePoint or OneDrive mount.
//
// It asks Graph for the folder's item with one GET. A 403 or a 404 is a definite no: Graph
// answers 403 for a library the person can't open, and 404 for a drive or folder they can't see
// or that doesn't exist. A rejected token (401), throttling and any other failure are errors,
// since they say nothing about access.
//
// baseURL is ignored. For an Entra connection it is the sign-in authority rather than Graph, and
// the driver's own Graph root (WithBaseURL) says where to ask.
func (d *Driver) CanRead(ctx context.Context, baseURL, source string, creds mounts.Credentials) (bool, error) {
	rest, ok := strings.CutPrefix(source, Scheme+"://")
	if !ok {
		return false, fmt.Errorf("%w: %q isn't a %s:// source", mounts.ErrInvalidArgument, source, Scheme)
	}
	name, key, _ := strings.Cut(rest, "/")
	b, err := d.connect(ctx, name, creds)
	if err != nil {
		return false, err
	}
	dr := b.(*drive)
	l, err := dr.split(key)
	if err != nil {
		return false, err
	}
	p := l.api + "/root?$select=id"
	if l.path != "" {
		p = l.api + "/root:/" + escapePath(l.path) + "?$select=id"
	}
	resp, err := dr.g.do(ctx, http.MethodGet, p, nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 8192))
		return true, nil
	case http.StatusForbidden, http.StatusNotFound:
		return false, nil
	}
	return false, status(resp, p)
}
