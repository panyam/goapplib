package mountstest

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	"github.com/panyam/goapplib/storage/mounts"
)

// DriverFiles are the files RunDriver asks a fixture's source to hold: a top-level file, two in a
// folder, and one two folders down, so ReadDir is checked at every depth.
var DriverFiles = map[string]string{
	"README.md":              "the jetson baseboard",
	"board/main.kicad_sch":   "(kicad_sch v1)",
	"board/power.kicad_sch":  "(kicad_sch power)",
	"docs/notes/assembly.md": "solder the big parts last",
}

// DriverFixture is one driver over one source that holds the files RunDriver asked for.
type DriverFixture struct {
	Driver mounts.Driver

	// Source holds the files the factory was given.
	Source string

	// Missing is a source of the driver's kind that doesn't exist, so Resolve returns ErrNotFound.
	Missing string

	// Creds are what the driver gets for every call. Nil means a public or local source.
	Creds mounts.Credentials

	// Change replaces the file at p in Source with data, as another writer would. A driver whose
	// source can't be changed from a test leaves it nil, and the checks that need it are skipped.
	Change func(t *testing.T, p, data string)

	// SnapshotFailsOnChange says a snapshot can't serve a file's old bytes after Change, as on a
	// bucket without versioning, so opening the changed file must fail with its path named rather
	// than serve the new bytes. False means a snapshot keeps serving the old bytes.
	SnapshotFailsOnChange bool
}

// DriverFactory returns a fixture whose Source holds files. Each call gets its own source.
type DriverFactory func(t *testing.T, files map[string]string) DriverFixture

// RunDriver checks that a driver keeps the contract the registry relies on: Resolve finds a
// source and refuses a missing one, and Open returns an fs.FS that passes fstest.TestFS with the
// source's files, for each mode the driver lists. A snapshot never serves new bytes as the
// version it pinned, and a live mount shows a change.
func RunDriver(t *testing.T, newFixture DriverFactory) {
	ctx := context.Background()

	t.Run("KindAndModes", func(t *testing.T) {
		f := newFixture(t, DriverFiles)
		if f.Driver.Kind() == "" || len(f.Driver.Modes()) == 0 {
			t.Fatalf("Kind = %q, Modes = %v; want both set", f.Driver.Kind(), f.Driver.Modes())
		}
	})

	t.Run("ResolveMissingIsNotFound", func(t *testing.T) {
		f := newFixture(t, DriverFiles)
		if _, err := f.Driver.Resolve(ctx, f.Missing, "", f.Creds); !errors.Is(err, mounts.ErrNotFound) {
			t.Fatalf("Resolve(%q) = %v, want ErrNotFound", f.Missing, err)
		}
	})

	for _, mode := range []mounts.Mode{mounts.ModeSnapshot, mounts.ModeLive} {
		t.Run(string(mode), func(t *testing.T) {
			f := newFixture(t, DriverFiles)
			if !slices.Contains(f.Driver.Modes(), mode) {
				t.Skipf("%s drivers don't support %s mounts", f.Driver.Kind(), mode)
			}
			m := open(t, f, mode)

			t.Run("TestFS", func(t *testing.T) {
				fsys, err := f.Driver.Open(ctx, m, f.Creds)
				if err != nil {
					t.Fatal(err)
				}
				if err := fstest.TestFS(fsys, slices.Sorted(maps.Keys(DriverFiles))...); err != nil {
					t.Fatal(err)
				}
				for p, want := range DriverFiles {
					if got, err := fs.ReadFile(fsys, p); err != nil || string(got) != want {
						t.Errorf("ReadFile(%q) = %q, %v; want %q", p, got, err, want)
					}
				}
			})

			t.Run("InvalidPathFails", func(t *testing.T) {
				fsys, err := f.Driver.Open(ctx, m, f.Creds)
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range []string{"../README.md", "/README.md", "board/../README.md"} {
					if _, err := fsys.Open(p); err == nil {
						t.Errorf("Open(%q) worked, want an error", p)
					}
				}
			})

			t.Run("AfterChange", func(t *testing.T) {
				if f.Change == nil {
					t.Skip("the fixture can't change its source")
				}
				f.Change(t, "README.md", "a new baseboard")
				fsys, err := f.Driver.Open(ctx, m, f.Creds)
				if err != nil {
					t.Fatal(err)
				}
				got, err := fs.ReadFile(fsys, "README.md")
				switch {
				case mode == mounts.ModeLive:
					if err != nil || string(got) != "a new baseboard" {
						t.Errorf("live ReadFile = %q, %v; want the change", got, err)
					}
				case f.SnapshotFailsOnChange:
					var pe *fs.PathError
					if err == nil || !errors.As(err, &pe) || pe.Path != "README.md" {
						t.Errorf("snapshot ReadFile = %q, %v; want a PathError naming README.md", got, err)
					}
				default:
					if err != nil || string(got) != DriverFiles["README.md"] {
						t.Errorf("snapshot ReadFile = %q, %v; want the pinned bytes", got, err)
					}
				}
				if mode == mounts.ModeSnapshot {
					if other, err := fs.ReadFile(fsys, "board/main.kicad_sch"); err != nil || string(other) != DriverFiles["board/main.kicad_sch"] {
						t.Errorf("an unchanged file = %q, %v; want its pinned bytes", other, err)
					}
				}
			})
		})
	}
}

// open makes a mount the way the registry's Create does: Resolve, then Prepare when the driver
// has it.
func open(t *testing.T, f DriverFixture, mode mounts.Mode) mounts.Mount {
	t.Helper()
	ctx := context.Background()
	m := mounts.Mount{Name: "contract", Kind: f.Driver.Kind(), Source: f.Source, Mode: mode, Created: time.Now()}
	v, err := f.Driver.Resolve(ctx, f.Source, "", f.Creds)
	if err != nil {
		t.Fatalf("Resolve(%q) = %v", f.Source, err)
	}
	m.Version = v
	if p, ok := f.Driver.(mounts.Preparer); ok {
		if m, err = p.Prepare(ctx, m, f.Creds); err != nil {
			t.Fatalf("Prepare = %v", err)
		}
	}
	return m
}
