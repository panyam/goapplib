package connections

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrInvalidGrant means a SourceGrant fails Validate: a missing source or viewer, or a viewer
// pattern with a '*' anywhere but its end.
var ErrInvalidGrant = errors.New("connections: invalid source grant")

// GrantedBySource is the connection ID CanRead returns when a SourceGrant admits the viewer. No
// connection can have this ID, since a store refuses a record whose ID fails Validate, and the ':'
// fails it. A caller compares the ID with GrantedBySource to tell a grant from a connection.
const GrantedBySource = "grant:"

// SourceGrant lets viewers read Source without a connection of their own. An app sets grants when
// the server's own access to a source stands for the people it signs in, such as a team's server
// whose operator has already chosen who may sign in (agni#1889).
//
// Source matches a CanRead source byte for byte, such as "github://acme/board". CanRead doesn't
// fold case, trim a trailing '/' or drop a ".git", so "github://Acme/board" doesn't match. A
// source spelled another way goes on to the viewer's connections as before.
//
// Each Viewers entry is one of three patterns:
//
//   - "*" matches any viewer.
//   - A prefix and a final '*', such as "local:*", matches a viewer that starts with the prefix
//     and has at least one more character, so "local:*" matches "local:carol" but not "local:".
//   - Anything else without a '*' matches that one viewer exactly.
//
// A pattern matches the viewer's subject, or any claim WithViewerClaims set, so "oidc:group:hw"
// admits a viewer whose claims include it. A grant never admits a request with no viewer, whatever
// its claims.
type SourceGrant struct {
	Source  string
	Viewers []string
}

// Validate checks that g has a source of the form scheme://rest with no '*', and at least one
// viewer pattern. A pattern is non-empty and has no '*' except as its last character. Every
// failure wraps ErrInvalidGrant and names the bad value, since a grant holds no secret.
func (g SourceGrant) Validate() error {
	scheme, rest, ok := strings.Cut(g.Source, "://")
	if !ok || scheme == "" || rest == "" {
		return fmt.Errorf("%w: source %q isn't scheme://rest", ErrInvalidGrant, g.Source)
	}
	if strings.Contains(g.Source, "*") {
		return fmt.Errorf("%w: source %q has a '*', but a source matches exactly", ErrInvalidGrant, g.Source)
	}
	if len(g.Viewers) == 0 {
		return fmt.Errorf("%w: source %q has no viewers", ErrInvalidGrant, g.Source)
	}
	for _, v := range g.Viewers {
		if !validViewerPattern(v) {
			return fmt.Errorf("%w: viewer pattern %q for %q must be \"*\", a prefix and a final '*', or one viewer",
				ErrInvalidGrant, v, g.Source)
		}
	}
	return nil
}

// validViewerPattern reports whether p is non-empty with no '*' before its last character.
func validViewerPattern(p string) bool {
	return p != "" && !strings.Contains(p[:len(p)-1], "*")
}

// matchViewer reports whether viewer matches pattern p. An invalid pattern matches nobody.
func matchViewer(p, viewer string) bool {
	if viewer == "" || !validViewerPattern(p) {
		return false
	}
	if prefix, ok := strings.CutSuffix(p, "*"); ok {
		return len(viewer) > len(prefix) && strings.HasPrefix(viewer, prefix)
	}
	return viewer == p
}

// WithSourceGrants sets the grants CanRead checks before the viewer's connections. A viewer that
// matches a grant on the source gets GrantedBySource without a read check, even on a service with
// no read check for that scheme. Grants for one source add up.
//
// The service copies the grants, so they are fixed once NewService returns, and a later
// WithSourceGrants in the same call replaces them. WithSourceGrants doesn't validate. A pattern that
// fails Validate matches nobody, so an app calls Validate on each grant first to report a typo.
func WithSourceGrants(grants ...SourceGrant) Option {
	bySource := map[string][]string{}
	for _, g := range grants {
		bySource[g.Source] = append(bySource[g.Source], g.Viewers...)
	}
	return func(s *Service) { s.grants = bySource }
}

// granted reports whether a grant on source admits viewer, by the subject or one of its claims.
// viewer is never "" here, since CanRead refuses a request with no viewer first.
func (s *Service) granted(viewer string, claims []string, source string) bool {
	return slices.ContainsFunc(s.grants[source], func(p string) bool {
		return matchViewer(p, viewer) || slices.ContainsFunc(claims, func(c string) bool { return matchViewer(p, c) })
	})
}
