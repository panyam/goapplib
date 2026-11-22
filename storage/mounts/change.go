package mounts

import (
	"context"
	"fmt"
)

// Change says that a ref in a source moved, such as a push to a branch that a host's webhook
// reported (goapplib issue 179). A driver's package turns its host's event into a Change, as
// github.ParsePush does, so an app's handler finds the mounts to check with Registry.Following and
// never reads the host's own format.
type Change struct {
	// Source is the source that changed, in the driver's form, such as "github://owner/repo".
	Source string

	// Ref is the short name of the ref that moved, such as "main" or "v1.2.0".
	Ref string

	// RefKind is RefBranch or RefTag.
	RefKind RefKind

	// Version is what Ref points at now, in the format the driver's Resolve returns. It is empty
	// when Deleted is true.
	Version string

	// Before is what Ref pointed at before, empty when the ref is new.
	Before string

	// Deleted is true when the ref was removed.
	Deleted bool

	// Default is true when Ref is the source's default branch, so a mount whose Ref is empty
	// follows it.
	Default bool
}

// SourceMatcher is a Driver that says when two of its sources name the same place, when plain
// string equality isn't enough. GitHub's owner and repo names ignore case, for one.
type SourceMatcher interface {
	Driver

	// SameSource reports whether a and b name the same source.
	SameSource(a, b string) bool
}

// SourceContainer is a Driver whose change events name something inside a source, such as one
// object under a bucket prefix, so a change matches every mount whose source holds it. A bucket
// event names "s3://designs/boards/jetson/README.md", and a mount of "s3://designs/boards" or
// "s3://designs" follows it.
type SourceContainer interface {
	Driver

	// ContainsSource reports whether changed, the Source of a Change, is mountSource or lies
	// under it. It works on whole path segments, so "s3://b/ab" isn't under "s3://b/a".
	ContainsSource(mountSource, changed string) bool
}

// Following returns the mounts of kind whose source is c.Source and whose ref is c.Ref, or empty
// when c is a change to the default branch. For a SourceContainer driver, a mount follows a change
// whose source lies under the mount's, and a bucket change's empty Ref matches a bucket mount's.
// Snapshot and live mounts both match, since an app may want to move a snapshot's links forward. A
// mount pinned to a commit never matches. A kind with no driver returns ErrInvalidArgument.
func (r *Registry) Following(ctx context.Context, kind Kind, c Change) ([]Mount, error) {
	d, ok := r.drivers[kind]
	if !ok {
		return nil, fmt.Errorf("%w: no driver for kind %q", ErrInvalidArgument, kind)
	}
	same := func(a, b string) bool { return a == b }
	if sc, ok := d.(SourceContainer); ok {
		same = sc.ContainsSource
	} else if sm, ok := d.(SourceMatcher); ok {
		same = sm.SameSource
	}
	all, err := r.List(ctx)
	if err != nil {
		return nil, err
	}
	var out []Mount
	for _, m := range all {
		if m.Kind != kind || !same(m.Source, c.Source) {
			continue
		}
		if m.Ref == c.Ref || m.Ref == "" && c.Default && c.RefKind == RefBranch {
			out = append(out, m)
		}
	}
	return out, nil
}
