package blobs_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/blobs/blobstest"
)

func TestDirKeepsTheStoreContract(t *testing.T) {
	blobstest.RunStore(t, func(t *testing.T) blobstest.Fixture {
		root := t.TempDir()
		d, err := blobs.NewDir(root)
		if err != nil {
			t.Fatal(err)
		}
		return blobstest.Fixture{
			Store: d,
			Later: func(t *testing.T, by time.Duration) {
				err := filepath.WalkDir(root, func(p string, de fs.DirEntry, err error) error {
					if err != nil || de.IsDir() {
						return err
					}
					fi, err := de.Info()
					if err != nil {
						return err
					}
					old := fi.ModTime().Add(-by)
					return os.Chtimes(p, old, old)
				})
				if err != nil {
					t.Fatal(err)
				}
			},
			Corrupt: func(t *testing.T, h blobs.Hash, b []byte) {
				p, err := d.Path(h)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, b, 0o600); err != nil {
					t.Fatal(err)
				}
			},
		}
	})
}
