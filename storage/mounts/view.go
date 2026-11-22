package mounts

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/panyam/goapplib/storage/blobs"
)

// BuildView makes view, a folder on this server's disk holding each file in files as a link to
// its blob in l, unless view already exists. A snapshot driver's HostPath calls it. files is
// called only when view is missing, so a driver can read its manifest lazily.
//
// It builds in a temp folder beside view and renames, so nobody sees a half-built view, and two
// processes building at once leave one whole view. Each entry's Path must be clean and relative,
// as blobs.GetManifest checks. A blob that l doesn't hold fails the build with blobs.ErrNotFound.
func BuildView(view string, l blobs.Linker, files func() (blobs.Manifest, error)) error {
	if _, err := os.Stat(view); err == nil {
		return nil
	}
	man, err := files()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(view), 0o750); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(view), ".build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	for _, e := range man {
		p, err := CleanPath(e.Path)
		if err != nil || p == "" {
			return fmt.Errorf("%w: view file %q", ErrInvalidPath, e.Path)
		}
		to := filepath.Join(tmp, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(to), 0o750); err != nil {
			return err
		}
		if err := l.Link(e.Hash, to); err != nil {
			return fmt.Errorf("%s: %w", e.Path, err)
		}
	}
	if err := os.Rename(tmp, view); err != nil {
		if _, statErr := os.Stat(view); statErr == nil {
			return nil
		}
		return err
	}
	return nil
}
