package mounts

import (
	"fmt"
	"path"
	"strings"
)

// Scheme is the URI scheme for a file in a mount.
const Scheme = "mount://"

// URI names a file or folder in a mount: mount://<Mount>/<Path>. Path is clean and relative, and
// never leaves the mount, so resolving it against the mount's root can't reach a file outside it.
// The empty Path is the mount's root.
type URI struct {
	Mount string
	Path  string
}

// ValidName reports whether name can name a mount. A name is 1 to 64 characters. The first is
// a-z or 0-9, and the rest may also be '-', '_' or '.'. The rule keeps names safe as a folder name,
// a URL path element and a YAML file name on any OS. It returns an error wrapping
// ErrInvalidArgument.
func ValidName(name string) error {
	if len(name) == 0 || len(name) > 64 {
		return fmt.Errorf("%w: mount name %q must be 1 to 64 characters", ErrInvalidArgument, name)
	}
	for i, c := range name {
		ok := c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
		if i > 0 {
			ok = ok || c == '-' || c == '_' || c == '.'
		}
		if !ok {
			return fmt.Errorf("%w: mount name %q may use only a-z, 0-9, and -_. after the first character", ErrInvalidArgument, name)
		}
	}
	return nil
}

// NewURI builds a URI from a mount name and a path inside it. The path may use '\' as a separator
// and may end in '/'. It may not be absolute or climb out with "..". Both failures wrap
// ErrInvalidPath, and a bad name wraps ErrInvalidArgument.
func NewURI(mount, p string) (URI, error) {
	if err := ValidName(mount); err != nil {
		return URI{}, err
	}
	clean, err := CleanPath(p)
	if err != nil {
		return URI{}, err
	}
	return URI{Mount: mount, Path: clean}, nil
}

// CleanPath applies NewURI's path rules to p, a path inside a mount. It returns the clean,
// relative, slash-separated form, and "" for the mount's root. An absolute path, or one that climbs
// out with "..", wraps ErrInvalidPath. Drivers call it on every path they are given, so a path
// that skipped NewURI still can't leave the mount.
func CleanPath(p string) (string, error) {
	p = strings.ReplaceAll(strings.TrimSpace(p), `\`, "/")
	if strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("%w: %q is absolute", ErrInvalidPath, p)
	}
	if p == "" {
		return "", nil
	}
	clean := path.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: %q leaves the mount", ErrInvalidPath, p)
	}
	if clean == "." {
		clean = ""
	}
	return clean, nil
}

// ParseURI parses "mount://<name>/<path>" with the rules of NewURI. Any other scheme wraps
// ErrInvalidPath.
func ParseURI(s string) (URI, error) {
	rest, ok := strings.CutPrefix(s, Scheme)
	if !ok {
		return URI{}, fmt.Errorf("%w: %q is not a %s URI", ErrInvalidPath, s, Scheme)
	}
	name, p, _ := strings.Cut(rest, "/")
	return NewURI(name, p)
}

// String formats the URI as mount://<Mount>/<Path>, which ParseURI reads back.
func (u URI) String() string {
	return Scheme + u.Mount + "/" + u.Path
}

// Join returns the URI of elem under u, with the rules of NewURI.
func (u URI) Join(elem ...string) (URI, error) {
	return NewURI(u.Mount, path.Join(append([]string{u.Path}, elem...)...))
}
