package folder_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/panyam/goapplib/storage/mounts"
	"github.com/panyam/goapplib/storage/mounts/folder"
)

const pointer = "version https://git-lfs.github.com/spec/v1\noid sha256:4d7a214614ab2935c943f9e0ff69d22eadbb8f32b1258daaa5e2ca24d17e2393\nsize 48213344\n"

func TestLFSPointerIsRefusedWithThePathNamed(t *testing.T) {
	m := tree(t)
	must(t, os.MkdirAll(filepath.Join(m.Source, "designs"), 0o755))
	must(t, os.WriteFile(filepath.Join(m.Source, "designs", "big.kicad_pcb"), []byte(pointer), 0o644))
	d := folder.New()
	fsys, err := d.Open(context.Background(), m, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := fs.ReadFile(fsys, "designs/big.kicad_pcb")
	if !errors.Is(err, mounts.ErrLFS) || !strings.Contains(err.Error(), "designs/big.kicad_pcb") {
		t.Errorf("ReadFile(pointer) = %q, %v; want ErrLFS naming the path", b, err)
	}
	if b, err := fs.ReadFile(fsys, "a.txt"); err != nil || string(b) != "alpha" {
		t.Errorf("ReadFile(a.txt) = %q, %v; a small plain file should read whole", b, err)
	}
	if _, err := d.HostPath(m, "designs/big.kicad_pcb"); !errors.Is(err, mounts.ErrLFS) {
		t.Errorf("HostPath(pointer) = %v, want ErrLFS", err)
	}
	if _, err := d.HostPath(m, "designs"); err != nil {
		t.Errorf("HostPath(folder) = %v; a folder isn't checked", err)
	}
}
