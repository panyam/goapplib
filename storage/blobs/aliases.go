package blobs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Aliases is a Store that also remembers which blob an outside ID names. Each ID lives in a
// namespace, such as "git" for git blob ids. A driver that lists a source's files with their own
// content IDs uses it to skip downloading a file the store already holds.
//
// An alias is a hint. The blob it names can be swept, so a caller checks Has before trusting it.
type Aliases interface {
	Store

	// Alias returns the hash id names in ns, and false when it names none.
	Alias(ctx context.Context, ns, id string) (Hash, bool, error)

	// SetAlias records that id in ns names h, replacing any earlier alias.
	SetAlias(ctx context.Context, ns, id string, h Hash) error
}

// aliasPart is what a namespace or an ID may hold, so neither can name a path outside the folder.
var aliasPart = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

// ErrInvalidAlias means an alias's namespace or ID isn't letters, digits, '_', '.' and '-', or is
// "." or "..".
var ErrInvalidAlias = errors.New("blobs: invalid alias")

func checkAlias(ns, id string) error {
	for _, s := range []string{ns, id} {
		if !aliasPart.MatchString(s) || s == "." || s == ".." {
			return fmt.Errorf("%w: %q", ErrInvalidAlias, s)
		}
	}
	return nil
}

// aliases is the folder a Dir keeps its aliases in, <root>/aliases, beside <root>/sha256, so Sweep
// never walks it.
func (d *Dir) aliases() string { return filepath.Join(filepath.Dir(d.root), "aliases") }

// Alias implements Aliases. An alias file that doesn't hold a valid hash counts as none.
func (d *Dir) Alias(ctx context.Context, ns, id string) (Hash, bool, error) {
	if err := checkAlias(ns, id); err != nil {
		return "", false, err
	}
	b, err := os.ReadFile(filepath.Join(d.aliases(), ns, id)) // #nosec G304 -- ns and id pass checkAlias
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	h, err := ParseHash(strings.TrimSpace(string(b)))
	if err != nil {
		return "", false, nil
	}
	return h, true, nil
}

// SetAlias implements Aliases. It writes through a temp file and a rename, so a reader sees the
// old alias or the new one.
func (d *Dir) SetAlias(ctx context.Context, ns, id string, h Hash) error {
	if err := checkAlias(ns, id); err != nil {
		return err
	}
	if _, err := ParseHash(string(h)); err != nil {
		return err
	}
	dir := filepath.Join(d.aliases(), ns)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".put-*")
	if err != nil {
		return err
	}
	if _, err := tmp.WriteString(string(h)); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), filepath.Join(dir, id)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

var _ Aliases = (*Dir)(nil)
