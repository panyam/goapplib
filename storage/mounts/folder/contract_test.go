package folder_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/panyam/goapplib/storage/blobs"
	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/folder"
	"github.com/panyam/goapplib/storage/mounts/mountstest"
)

func TestDriverContract(t *testing.T) {
	mountstest.RunDriver(t, func(t *testing.T, files map[string]string) mountstest.DriverFixture {
		dir := t.TempDir()
		write := func(t *testing.T, p, data string) {
			to := filepath.Join(dir, filepath.FromSlash(p))
			if err := os.MkdirAll(filepath.Dir(to), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(to, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		for p, data := range files {
			write(t, p, data)
		}
		store, err := blobs.NewDir(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return mountstest.DriverFixture{
			Driver:    folder.New(folder.WithBlobs(store)),
			Source:    dir,
			Missing:   filepath.Join(dir, "nope"),
			Change:    write,
			HostPaths: []mounts.Mode{mounts.ModeLive},
		}
	})
}
