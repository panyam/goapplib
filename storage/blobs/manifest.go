package blobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
)

// ManifestEntry is one file in a Manifest.
type ManifestEntry struct {
	// Path is the file's place in the view: relative, clean and slash-separated.
	Path string `json:"path"`

	Hash Hash  `json:"hash"`
	Size int64 `json:"size"`
}

// Manifest lists the files of a view, such as a mount snapshot, and the blob holding each. It is
// stored as a blob itself, so a record names a whole snapshot with one hash however many files it
// has.
type Manifest []ManifestEntry

func (m Manifest) check() error {
	seen := map[string]bool{}
	for _, e := range m {
		p := e.Path
		if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, `\`) || path.Clean(p) != p || p == ".." || strings.HasPrefix(p, "../") {
			return fmt.Errorf("%w: path %q", ErrInvalidManifest, p)
		}
		if seen[p] {
			return fmt.Errorf("%w: %q appears twice", ErrInvalidManifest, p)
		}
		seen[p] = true
		if _, err := hexOf(e.Hash); err != nil {
			return fmt.Errorf("%w: %q: %v", ErrInvalidManifest, p, err)
		}
	}
	return nil
}

// PutManifest checks m and stores it in s, sorted by path, so the same files give the same hash
// in any order. It doesn't check that s holds the files' blobs. It returns ErrInvalidManifest
// for a bad path or hash.
func PutManifest(ctx context.Context, s Store, m Manifest) (Hash, error) {
	if err := m.check(); err != nil {
		return "", err
	}
	sorted := append(Manifest(nil), m...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	b, err := json.Marshal(sorted)
	if err != nil {
		return "", err
	}
	return s.Put(ctx, bytes.NewReader(b))
}

// GetManifest reads the manifest stored under h, checking it as PutManifest does.
func GetManifest(ctx context.Context, s Store, h Hash) (Manifest, error) {
	r, err := s.Open(ctx, h)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidManifest, h, err)
	}
	return m, m.check()
}

// ManifestRefs calls keep for the manifest h and for every file it lists, which is what a
// Referrer for a snapshot needs to pass to Sweep.
func ManifestRefs(ctx context.Context, s Store, h Hash, keep func(Hash)) error {
	m, err := GetManifest(ctx, s, h)
	if err != nil {
		return err
	}
	keep(h)
	for _, e := range m {
		keep(e.Hash)
	}
	return nil
}
