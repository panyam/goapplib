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
//
// A URI's mount name follows a looser rule than ValidName: any name that isn't empty, ".", "..",
// and holds no '/' or '\'. An app names a fixed mount after a folder, such as agni's --mount-root,
// and a folder can have capitals or spaces. ValidName still governs the runtime mounts a record
// store keeps.
func NewURI(mount, p string) (URI, error) {
	if mount == "" || mount == "." || mount == ".." || strings.ContainsAny(mount, `/\`) {
		return URI{}, fmt.Errorf("%w: mount name %q must be non-empty and hold no separator", ErrInvalidArgument, mount)
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

// String formats the URI as mount://<Mount>/<Path>, or mount://<Mount> for the mount's root, which
// ParseURI reads back.
func (u URI) String() string {
	if u.Path == "" {
		return Scheme + u.Mount
	}
	return Scheme + u.Mount + "/" + u.Path
}

// IsZero reports whether the URI names nothing, as an absent optional field does.
func (u URI) IsZero() bool { return u.Mount == "" && u.Path == "" }

// Dir returns the URI of the folder holding u, or u itself for the mount's root.
func (u URI) Dir() URI {
	if u.Path == "" {
		return u
	}
	d := path.Dir(u.Path)
	if d == "." {
		d = ""
	}
	return URI{Mount: u.Mount, Path: d}
}

// Base returns the last element of the path, as path.Base does, and "" for the mount's root.
func (u URI) Base() string {
	if u.Path == "" {
		return ""
	}
	return path.Base(u.Path)
}

// Resolve reads ref relative to u, the way a browser reads an href against the page it is on. A
// reader uses it when a file names another, such as a schematic naming its child sheets.
//
// A full mount:// URI replaces u, mount included. "/x" names x from the mount's root. Anything else
// resolves against u's folder, so "sub/a.kicad_sch" against mount://m/designs/top.kicad_sch is
// mount://m/designs/sub/a.kicad_sch. A result that would climb out of the mount wraps
// ErrInvalidPath. Unlike RFC 3986, a leading ".." on an absolute path is an error rather than
// dropped, since quietly reading "/../x" as "/x" would hide a traversal attempt.
func (u URI) Resolve(ref string) (URI, error) {
	switch {
	case strings.HasPrefix(ref, Scheme):
		return ParseURI(ref)
	case strings.HasPrefix(ref, "/") || strings.HasPrefix(ref, `\`):
		return NewURI(u.Mount, strings.TrimLeft(ref, `/\`))
	default:
		return NewURI(u.Mount, path.Join(u.Dir().Path, strings.ReplaceAll(ref, `\`, "/")))
	}
}

// Join returns the URI of elem under u, with the rules of NewURI.
func (u URI) Join(elem ...string) (URI, error) {
	return NewURI(u.Mount, path.Join(append([]string{u.Path}, elem...)...))
}
