package mounts

import (
	"bytes"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// ErrLFS means a file is a Git LFS pointer whose object the driver couldn't read. The error also
// wraps the reason where there is one (ErrNotFound for a missing object, ErrPermissionDenied for no
// LFS access, ErrUnavailable for a quota or a network failure), and its message names the path. A
// driver never serves the pointer in place of the file.
var ErrLFS = errors.New("mounts: Git LFS object not available")

// MaxLFSPointerSize is the largest file ParseLFSPointer looks at. Git LFS writes pointers well
// under it, so a driver checks only files this small.
const MaxLFSPointerSize = 1024

// LFSPointer is a parsed Git LFS pointer: the SHA-256 of the object's contents, in lowercase
// hex, and its size in bytes.
type LFSPointer struct {
	OID  string
	Size int64
}

var lfsOID = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ParseLFSPointer reports whether b is a Git LFS pointer, as the spec at
// https://github.com/git-lfs/git-lfs/blob/main/docs/spec.md defines it: a first line of
// "version https://git-lfs.github.com/spec/v1", then "oid sha256:<hex>" and "size <bytes>" lines.
// A file over MaxLFSPointerSize is never a pointer.
func ParseLFSPointer(b []byte) (LFSPointer, bool) {
	if len(b) > MaxLFSPointerSize || !bytes.HasPrefix(b, []byte("version https://git-lfs.github.com/spec/v1\n")) {
		return LFSPointer{}, false
	}
	var p LFSPointer
	sized := false
	for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n")[1:] {
		k, v, ok := strings.Cut(line, " ")
		if !ok {
			return LFSPointer{}, false
		}
		switch k {
		case "oid":
			hex, ok := strings.CutPrefix(v, "sha256:")
			if !ok || !lfsOID.MatchString(hex) {
				return LFSPointer{}, false
			}
			p.OID = hex
		case "size":
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil || n < 0 {
				return LFSPointer{}, false
			}
			p.Size, sized = n, true
		}
	}
	return p, p.OID != "" && sized
}
