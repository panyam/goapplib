package page

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// testdata/esbuild-meta.json is real esbuild output: web/main.ts dynamically imports
// islands/{hero,below,narrow}.ts, hero and narrow share chunk-I2CYLLOU (which imports
// chunk-M4DPGN35), and all three share chunk-M4DPGN35.
func loadFixture(t *testing.T, opts EsbuildOptions) *Assets {
	t.Helper()
	a, err := LoadEsbuildMetafile(filepath.Join("testdata", "esbuild-meta.json"), opts)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestEsbuildMetafileNamesEachDynamicChunkByItsSourceFile(t *testing.T) {
	a := loadFixture(t, EsbuildOptions{OutDir: "dist", URLPrefix: "/static/"})
	want := map[string]Chunk{
		"hero":   {File: "/static/chunks/hero-HVBOBZWN.js", Imports: []string{"/static/chunks/chunk-I2CYLLOU.js", "/static/chunks/chunk-M4DPGN35.js"}},
		"below":  {File: "/static/chunks/below-6H45KFPN.js", Imports: []string{"/static/chunks/chunk-M4DPGN35.js"}},
		"narrow": {File: "/static/chunks/narrow-6K72O4OZ.js", Imports: []string{"/static/chunks/chunk-I2CYLLOU.js", "/static/chunks/chunk-M4DPGN35.js"}},
	}
	if !reflect.DeepEqual(a.Islands, want) {
		t.Fatalf("islands:\n got %#v\nwant %#v", a.Islands, want)
	} // web/main.ts reaches the islands only through import(), so the entry imports nothing to preload.
	if got, want := a.Entries, map[string]Chunk{"main": {File: "/static/main.js"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("entries: got %#v, want %#v", got, want)
	}
}

func TestForPreloadsOnlyTheEagerIslandsAndWhatTheyImport(t *testing.T) {
	a := loadFixture(t, EsbuildOptions{OutDir: "dist", URLPrefix: "/static/"})
	spec := Spec{Layout: "x", Islands: []Island{
		{Name: "below", Slot: "bottom", Load: "visible"},
		{Name: "hero", Slot: "top"},
		{Name: "narrow", Slot: "side", Load: "media:(max-width: 600px)"},
		{Name: "inline", Slot: "foot", Load: "eager"}, // not a chunk of its own
	}}
	want := []string{"/static/chunks/hero-HVBOBZWN.js", "/static/chunks/chunk-I2CYLLOU.js", "/static/chunks/chunk-M4DPGN35.js"}
	if got := a.For("main", spec); !reflect.DeepEqual(got, want) {
		t.Fatalf("For: got %v, want %v", got, want)
	}

	spec.Islands[0].Load = "eager"
	want = []string{"/static/chunks/below-6H45KFPN.js", "/static/chunks/chunk-M4DPGN35.js", "/static/chunks/hero-HVBOBZWN.js", "/static/chunks/chunk-I2CYLLOU.js"}
	if got := a.For("main", spec); !reflect.DeepEqual(got, want) {
		t.Fatalf("For with below eager: got %v, want %v (spec order, each chunk once)", got, want)
	}
}

func TestForOnNilAssetsIsEmpty(t *testing.T) {
	var a *Assets
	if got := a.For("main", Spec{Layout: "x", Islands: []Island{{Name: "hero", Slot: "top"}}}); len(got) != 0 {
		t.Fatalf("nil Assets: got %v", got)
	}
}

func TestEsbuildMetafileFollowsImportsThroughOtherChunks(t *testing.T) {
	dir := t.TempDir()
	meta := `{"outputs": {
		"out/main.js": {"imports": [{"path": "out/c/tools-1.js", "kind": "dynamic-import"}]},
		"out/c/tools-1.js": {"entryPoint": "src/islands/Tools.tsx", "imports": [{"path": "out/c/chunk-a.js", "kind": "import-statement"}, {"path": "out/c/more-2.js", "kind": "dynamic-import"}]},
		"out/c/chunk-a.js": {"imports": [{"path": "out/c/chunk-b.js", "kind": "import-statement"}]},
		"out/c/chunk-b.js": {"imports": [{"path": "out/c/chunk-a.js", "kind": "import-statement"}]},
		"out/c/more-2.js": {"entryPoint": "src/more.ts", "imports": []}
	}}`
	file := filepath.Join(dir, "meta.json")
	if err := os.WriteFile(file, []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := LoadEsbuildMetafile(file, EsbuildOptions{OutDir: "out/", URLPrefix: "/s/", Name: func(src string) string {
		if strings.Contains(src, "/islands/") {
			return strings.ToLower(strings.TrimSuffix(filepath.Base(src), ".tsx"))
		}
		return ""
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Chunk{"tools": {File: "/s/c/tools-1.js", Imports: []string{"/s/c/chunk-a.js", "/s/c/chunk-b.js"}}}
	if !reflect.DeepEqual(a.Islands, want) {
		t.Fatalf("got %#v, want %#v", a.Islands, want)
	}
}

func TestEsbuildMetafileErrors(t *testing.T) {
	dir := t.TempDir()
	for name, meta := range map[string]string{
		"not json": `{"outputs": `,
		"two chunks, one name": `{"outputs": {
			"d/main.js": {"imports": [{"path": "d/a.js", "kind": "dynamic-import"}, {"path": "d/b.js", "kind": "dynamic-import"}]},
			"d/a.js": {"entryPoint": "x/hero.ts"},
			"d/b.js": {"entryPoint": "y/hero.ts"}
		}}`,
	} {
		file := filepath.Join(dir, strings.ReplaceAll(name, " ", "_"))
		if err := os.WriteFile(file, []byte(meta), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadEsbuildMetafile(file, EsbuildOptions{}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if _, err := LoadEsbuildMetafile(filepath.Join(dir, "missing.json"), EsbuildOptions{}); err == nil {
		t.Error("missing file: no error")
	}
}

func TestLoadAssetsReadsTheCommonShape(t *testing.T) {
	file := filepath.Join(t.TempDir(), "assets.json")
	if err := os.WriteFile(file, []byte(`{"islands": {"hero": {"file": "/a/hero.js", "imports": ["/a/x.js"]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := LoadAssets(file)
	if err != nil {
		t.Fatal(err)
	}
	if got := a.For("main", Spec{Layout: "x", Islands: []Island{{Name: "hero", Slot: "top"}}}); !reflect.DeepEqual(got, []string{"/a/hero.js", "/a/x.js"}) {
		t.Fatalf("For: %v", got)
	}
}

func TestForPreloadsTheEntrysChunksThenTheEagerIslands(t *testing.T) {
	dir := t.TempDir()
	// main shares tsappkit's core with the below island, so esbuild splits it into chunk-core,
	// which main imports statically; chunk-core in turn imports chunk-util.
	meta := `{"outputs": {
		"dist/main.js": {"entryPoint": "web/main.ts", "imports": [
			{"path": "dist/c/chunk-core.js", "kind": "import-statement"},
			{"path": "dist/c/hero-1.js", "kind": "dynamic-import"},
			{"path": "dist/c/below-2.js", "kind": "dynamic-import"}]},
		"dist/admin.js": {"entryPoint": "web/admin.ts", "imports": [{"path": "dist/c/chunk-admin.js", "kind": "import-statement"}]},
		"dist/c/chunk-core.js": {"imports": [{"path": "dist/c/chunk-util.js", "kind": "import-statement"}]},
		"dist/c/chunk-util.js": {"imports": []},
		"dist/c/chunk-admin.js": {"imports": []},
		"dist/c/hero-1.js": {"entryPoint": "web/islands/hero.ts", "imports": [{"path": "dist/c/chunk-util.js", "kind": "import-statement"}]},
		"dist/c/below-2.js": {"entryPoint": "web/islands/below.tsx", "imports": [{"path": "dist/c/chunk-core.js", "kind": "import-statement"}]}
	}}`
	file := filepath.Join(dir, "meta.json")
	if err := os.WriteFile(file, []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := LoadEsbuildMetafile(file, EsbuildOptions{OutDir: "dist", URLPrefix: "/static/"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := a.Entries["main"], (Chunk{File: "/static/main.js", Imports: []string{"/static/c/chunk-core.js", "/static/c/chunk-util.js"}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("main: got %#v, want %#v", got, want)
	}
	if got, want := a.Names(), []string{"below", "hero"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("islands %v, want %v (entries aren't islands)", got, want)
	}

	spec := Spec{Layout: "x", Islands: []Island{{Name: "hero", Slot: "top"}, {Name: "below", Slot: "bottom", Load: "visible"}}}
	want := []string{"/static/c/chunk-core.js", "/static/c/chunk-util.js", "/static/c/hero-1.js"}
	if got := a.For("main", spec); !reflect.DeepEqual(got, want) {
		t.Fatalf("For(main): got %v, want %v (the entry's chunks, then hero's, each once; not below's, not admin's)", got, want)
	}
	if got, want := a.For("nosuch", spec), []string{"/static/c/hero-1.js", "/static/c/chunk-util.js"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("For(unknown entry): got %v, want only the islands' %v", got, want)
	}
}

func TestEsbuildMetafileRejectsTwoEntriesWithOneName(t *testing.T) {
	file := filepath.Join(t.TempDir(), "meta.json")
	meta := `{"outputs": {"d/a/main.js": {"entryPoint": "a/main.ts"}, "d/b/main.js": {"entryPoint": "b/main.ts"}}}`
	if err := os.WriteFile(file, []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEsbuildMetafile(file, EsbuildOptions{}); err == nil || !strings.Contains(err.Error(), `entry "main"`) {
		t.Fatalf("want an error naming entry \"main\", got %v", err)
	}
}
