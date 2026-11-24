package connections

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/panyam/goapplib/storage/mounts"
)

// ErrNoAccess means none of the viewer's connections can read the source, or the viewer has
// none to that service.
var ErrNoAccess = errors.New("connections: no connection of the viewer's can read this source")

// DefaultAccessTTL is how long CanRead keeps an answer for one viewer and source.
const DefaultAccessTTL = time.Minute

// ReadCheck asks a service whether creds can read source, using the API at baseURL. It returns false
// with a nil error for a definite no, such as GitHub's 404 for a private repo, and also when
// baseURL isn't the host it serves. Any other failure is an error. The GitHub driver's CanRead
// method is one.
type ReadCheck func(ctx context.Context, baseURL, source string, creds mounts.Credentials) (bool, error)

// WithReadCheck sets the read check CanRead uses for a source whose scheme has none from
// WithReadCheckFor. It tries every one of the viewer's connections, so a token can reach a
// service it wasn't issued for: a KindEntraUser connection's token goes to GitHub, since an empty
// BaseURL means github.com to the GitHub check. Prefer WithReadCheckFor, which names the kinds.
// Without either, CanRead returns ErrNotConfigured.
func WithReadCheck(f ReadCheck) Option {
	return func(s *Service) { s.check = f }
}

// readCheck is a ReadCheck for one scheme and the connection kinds it may be tried with.
type readCheck struct {
	check ReadCheck
	kinds []Kind
}

// WithReadCheckFor sets the read check for sources with scheme, such as "github" or "msgraph",
// and the connection kinds CanRead tries with it. A viewer's connection of any other kind is
// skipped, so its token never reaches that service:
//
//	connections.WithReadCheckFor("github", githubDriver.CanRead, connections.KindGitHubUser, connections.KindGitHubToken)
//	connections.WithReadCheckFor("msgraph", graphDriver.CanRead, connections.KindEntraUser)
//
// No kinds means every kind, as WithReadCheck does. Setting a scheme again replaces its check.
func WithReadCheckFor(scheme string, f ReadCheck, kinds ...Kind) Option {
	return func(s *Service) {
		if s.checks == nil {
			s.checks = map[string]readCheck{}
		}
		s.checks[scheme] = readCheck{check: f, kinds: slices.Clone(kinds)}
	}
}

// readCheckFor returns the check for source and the kinds it may be tried with, or false.
func (s *Service) readCheckFor(source string) (readCheck, bool) {
	scheme, _, ok := strings.Cut(source, "://")
	if rc, found := s.checks[scheme]; ok && found {
		return rc, true
	}
	if s.check != nil {
		return readCheck{check: s.check}, true
	}
	return readCheck{}, false
}

// WithAccessTTL sets how long CanRead keeps an answer. Zero or less means DefaultAccessTTL.
func WithAccessTTL(d time.Duration) Option {
	return func(s *Service) { s.accessTTL = d }
}

type answer struct {
	id string
	at time.Time
}

func (a answer) result() (string, error) {
	if a.id == "" {
		return "", ErrNoAccess
	}
	return a.id, nil
}

// CanRead returns the ID of a connection of the viewer's that can read source, such as
// "github://acme/board". An app calls it to decide whether to show a person something it linked
// from a private repo, as agni#1728 does.
//
// A request with no viewer returns ErrNotOwner. Next, a SourceGrant on source that matches the
// viewer or one of its claims (WithViewerClaims) returns GrantedBySource with a nil error
// (WithSourceGrants). A grant answer asks no service
// and isn't kept, since the grants never change.
//
// Otherwise it picks the read check by the source's scheme (WithReadCheckFor), or the WithReadCheck one. It
// tries the viewer's own connections of that check's kinds in ID order, each with its credentials,
// and returns the first one the read check says can read. When none
// can, or the viewer has no connection, it returns ErrNoAccess. When a check failed and none
// succeeded, it returns that failure instead of ErrNoAccess, since the answer isn't known.
//
// A yes or a no is kept for the access TTL per viewer and source, so a page that checks many links
// asks GitHub once per repo. A failure isn't kept. SaveUserToken, SavePastedToken and Disconnect
// forget the owner's kept answers, and so does ForgetAccess. Server connections are never tried,
// since they would answer for the server rather than the person.
func (s *Service) CanRead(ctx context.Context, source string) (string, error) {
	viewer := Viewer(ctx)
	if viewer == "" {
		return "", fmt.Errorf("%w: CanRead needs a viewer", ErrNotOwner)
	}
	if s.granted(viewer, ViewerClaims(ctx), source) {
		return GrantedBySource, nil
	}
	rc, ok := s.readCheckFor(source)
	if !ok {
		return "", fmt.Errorf("%w: no read check for %q (WithReadCheckFor or WithReadCheck)", ErrNotConfigured, source)
	}
	key := viewer + "\x00" + source
	ttl := s.accessTTL
	if ttl <= 0 {
		ttl = DefaultAccessTTL
	}
	s.mu.Lock()
	a, ok := s.access[key]
	gen := s.gen[viewer]
	s.mu.Unlock()
	if ok && s.now().Sub(a.at) < ttl {
		return a.result()
	}

	conns, err := s.store.ListConnections(ctx, viewer)
	if err != nil {
		return "", err
	}
	var failed error
	found := ""
	for _, c := range conns {
		if len(rc.kinds) > 0 && !slices.Contains(rc.kinds, c.Kind) {
			continue
		}
		creds, err := s.Credentials(ctx, viewer, c.ID)
		if err == nil {
			var yes bool
			if yes, err = rc.check(ctx, c.BaseURL, source, creds); err == nil && yes {
				found = c.ID
				break
			}
		}
		if err != nil && failed == nil {
			failed = fmt.Errorf("connection %q: %w", c.ID, err)
		}
	}
	if found == "" && failed != nil {
		return "", failed
	}
	a = answer{id: found, at: s.now()}
	s.mu.Lock()
	if s.access == nil {
		s.access = map[string]answer{}
	}
	// A forget since this call began means the viewer's connections changed under it, so the
	// answer goes back to the caller without being kept.
	if s.gen[viewer] == gen {
		s.access[key] = a
	}
	s.mu.Unlock()
	return a.result()
}

// ForgetAccess drops every answer CanRead keeps for viewer, so the next call asks the service
// again. The service forgets on its own when SaveUserToken, SavePastedToken or Disconnect changes
// a person's connection. An app that adds, changes or removes a person's connection through the
// Store calls ForgetAccess after the write, or the person waits out the access TTL. A CanRead
// already running for viewer still returns its answer, but doesn't keep it.
func (s *Service) ForgetAccess(viewer string) {
	if viewer == "" {
		return
	}
	prefix := viewer + "\x00"
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen == nil {
		s.gen = map[string]uint64{}
	}
	s.gen[viewer]++
	for k := range s.access {
		if strings.HasPrefix(k, prefix) {
			delete(s.access, k)
		}
	}
}
