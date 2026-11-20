package mounts

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/oauth2"
)

// RefLister is a Driver that lists the versions its sources have, so an app can offer a list of
// revisions to link to (goapplib issue 177). agni lists a linked design's tags and a branch's
// commits with it. A driver whose sources have no history implements it and returns an empty page.
type RefLister interface {
	Driver

	// ListRefs lists source's tags, newest first by their commit's date, or the commits on one
	// branch, newest first. A source or branch that doesn't exist returns ErrNotFound. A request
	// the driver can't serve, such as tags with no token on a host that needs one, returns
	// ErrInvalidArgument.
	ListRefs(ctx context.Context, source string, req ListRefsRequest, ts oauth2.TokenSource) (RefPage, error)
}

// RefKind says what a ref names.
type RefKind string

const (
	// RefTag is a tag, which doesn't move.
	RefTag RefKind = "tag"

	// RefBranch is a branch, which moves with each commit on it.
	RefBranch RefKind = "branch"

	// RefCommit is one commit, named by its id.
	RefCommit RefKind = "commit"
)

// ListRefsRequest picks what ListRefs lists. Kind is RefTag or RefCommit. For RefCommit, Branch
// names the branch, and empty means the source's default. PageSize is a hint the driver may cap,
// and zero means the driver's default. PageToken is the NextPageToken of the page before, or empty
// for the first page.
type ListRefsRequest struct {
	Kind      RefKind
	Branch    string
	PageSize  int
	PageToken string
}

// RefPage is one page of refs. An empty NextPageToken means it is the last page.
type RefPage struct {
	Refs          []Ref
	NextPageToken string
}

// Ref is one tag or commit. Name is what a mount's Ref field takes to pin it: the tag's name, or
// the commit's id. Version is the commit Name resolves to, in the format Resolve returns, so an
// app can pass it to ListAt without resolving it again. Time is the commit's date. Message is the
// first line of the commit's message, and may be empty, as it is for a tag on some hosts.
type Ref struct {
	Name    string
	Kind    RefKind
	Version string
	Time    time.Time
	Message string
}

// ListRefs lists the tags, or a branch's commits, of the source of the mount named name, through
// its connection. A driver that can't list refs returns ErrInvalidArgument, and a Kind other than
// RefTag or RefCommit does too.
func (r *Registry) ListRefs(ctx context.Context, name string, req ListRefsRequest) (RefPage, error) {
	if req.Kind != RefTag && req.Kind != RefCommit {
		return RefPage{}, fmt.Errorf("%w: ListRefs lists tags or commits, not %q", ErrInvalidArgument, req.Kind)
	}
	if req.PageSize < 0 {
		return RefPage{}, fmt.Errorf("%w: page size %d", ErrInvalidArgument, req.PageSize)
	}
	m, err := r.Get(ctx, name)
	if err != nil {
		return RefPage{}, err
	}
	d, err := r.driver(m)
	if err != nil {
		return RefPage{}, err
	}
	rl, ok := d.(RefLister)
	if !ok {
		return RefPage{}, fmt.Errorf("%w: %s mounts can't list refs", ErrInvalidArgument, m.Kind)
	}
	ts, err := r.tokens(ctx, m)
	if err != nil {
		return RefPage{}, err
	}
	return rl.ListRefs(ctx, m.Source, req, ts)
}
