package blobs_test

import (
	"os"
	"testing"
	"time"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/blobs/blobstest"
)

func dirFixture(t *testing.T) blobstest.Fixture {
	root := t.TempDir()
	d, err := blobs.NewDir(root)
	if err != nil {
		t.Fatal(err)
	}
	return blobstest.Fixture{
		Store: d,
		Later: func(t *testing.T, by time.Duration) { blobstest.AgeDir(t, root, by) },
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
}

func TestDirKeepsTheStoreContract(t *testing.T) {
	blobstest.RunStore(t, dirFixture)
}

func TestDirKeepsTheLinkerContract(t *testing.T) {
	blobstest.RunLinker(t, dirFixture)
}

func TestCachedOverADir(t *testing.T) {
	blobstest.RunCached(t, dirFixture)
}
