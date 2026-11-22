package github

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/panyam/goapplib/storage/mounts"
)

// CanRead reports whether creds can read the repo at source. It fits connections.ReadCheck, so an app
// passes it to connections.WithReadCheck.
//
// baseURL is the connection's API root, empty for github.com. A connection to another host than
// this driver serves gets false with no request, since its token means nothing here. A 404 is a
// definite no, and it is also how GitHub answers a private repo the token can't see. Any other
// failure, a rejected token included, is an error, since it says nothing about access.
func (d *Driver) CanRead(ctx context.Context, baseURL, source string, creds mounts.Credentials) (bool, error) {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if strings.TrimRight(baseURL, "/") != d.base {
		return false, nil
	}
	rp, err := repo(source)
	if err != nil {
		return false, err
	}
	err = d.call(ctx, creds, http.MethodGet, rp, nil, nil, false, nil)
	if errors.Is(err, mounts.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}
