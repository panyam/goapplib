package page

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"
)

// Assets says which built files a page and its islands need, so a page can
// write <link rel="modulepreload"> for what it loads at once (see For and the
// IslandPreloads partial in templates/page/Islands.html). Each lazy island
// (tsappkit's lazy) is its own chunk, and code an island shares with the
// page's entry script goes into chunks the entry imports. Without preload
// links the browser only finds those chunks, and then their imports, after
// the entry has arrived and been parsed.
//
// The JSON form is the shape any bundler's build can write:
//
//	{"entries": {"main": {"file": "/static/main.js", "imports": ["/static/chunks/chunk-Z.js"]}},
//	 "islands": {"hero": {"file": "/static/chunks/hero-X.js", "imports": ["/static/chunks/chunk-Y.js"]}}}
//
// LoadAssets reads that; LoadEsbuildMetafile builds it from esbuild's metafile.
type Assets struct {
	// Entries are the page scripts (esbuild's entry points), by name.
	Entries map[string]Chunk `json:"entries"`
	// Islands are the lazy islands' chunks, by island name.
	Islands map[string]Chunk `json:"islands"`
}

// Chunk is one built script and what it needs. File is its URL; Imports are
// the URLs of every chunk it statically imports, directly or through another
// chunk, in a stable order. They never include a chunk reached only through a
// dynamic import(), which is another island and loads when that island does.
type Chunk struct {
	File    string   `json:"file"`
	Imports []string `json:"imports"`
}

// LoadAssets reads Assets in its JSON form from a file.
func LoadAssets(file string) (*Assets, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var a Assets
	if err := json.Unmarshal(b, &a); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return &a, nil
}

// EsbuildOptions says how LoadEsbuildMetafile turns the metafile's paths into
// entry and island names and URLs.
type EsbuildOptions struct {
	// OutDir is the output directory as the metafile writes it (the --outdir
	// relative to where esbuild ran, "dist" say). It's cut from the front of
	// each output path before URLPrefix is added.
	OutDir string
	// URLPrefix is where the page serves OutDir, such as "/static/".
	URLPrefix string
	// Name gives the entry or island name for an output's source file (its
	// path as the metafile has it, "web/islands/hero.ts"). The default is the
	// file's base name without its extension ("hero", or "main" for
	// web/main.ts), so a registry entry `hero: lazy(() => import("./islands/hero"))`
	// needs no setting. Returning "" leaves that output out.
	Name func(source string) string
}

type esbuildMetafile struct {
	Outputs map[string]struct {
		EntryPoint string `json:"entryPoint"`
		Imports    []struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		} `json:"imports"`
	} `json:"outputs"`
}

// LoadEsbuildMetafile builds Assets from the metafile of an esbuild build run
// with --splitting --format=esm --metafile. An output with a source file that
// some other output reaches through a dynamic import() is an island chunk;
// any other output with a source file is an entry. Both are named by
// opts.Name, and their Imports are the chunks they reach through static
// imports. Two islands, or two entries, with the same name are an error,
// since the page couldn't tell which one it means.
func LoadEsbuildMetafile(file string, opts EsbuildOptions) (*Assets, error) {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var m esbuildMetafile
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	name := opts.Name
	if name == nil {
		name = func(source string) string {
			base := path.Base(source)
			return strings.TrimSuffix(base, path.Ext(base))
		}
	}
	url := func(out string) string {
		rel := out
		if opts.OutDir != "" {
			rel = strings.TrimPrefix(strings.TrimPrefix(out, strings.TrimSuffix(opts.OutDir, "/")), "/")
		}
		return opts.URLPrefix + rel
	}
	chunk := func(out string) Chunk {
		var imports []string
		seen := map[string]bool{out: true}
		var visit func(string)
		visit = func(p string) {
			for _, imp := range m.Outputs[p].Imports {
				if imp.Kind != "import-statement" || seen[imp.Path] {
					continue
				}
				seen[imp.Path] = true
				imports = append(imports, url(imp.Path))
				visit(imp.Path)
			}
		}
		visit(out)
		return Chunk{File: url(out), Imports: imports}
	}

	dynamic := map[string]bool{}
	for _, o := range m.Outputs {
		for _, imp := range o.Imports {
			if imp.Kind == "dynamic-import" {
				dynamic[imp.Path] = true
			}
		}
	}
	a := &Assets{Entries: map[string]Chunk{}, Islands: map[string]Chunk{}}
	from := map[string]string{}
	for out, o := range m.Outputs {
		if o.EntryPoint == "" {
			continue
		}
		n := name(o.EntryPoint)
		if n == "" {
			continue
		}
		kind, into := "entry", a.Entries
		if dynamic[out] {
			kind, into = "island", a.Islands
		}
		if prev, ok := from[kind+" "+n]; ok {
			return nil, fmt.Errorf("%s: %s %q has two outputs, from %s and %s", file, kind, n, prev, o.EntryPoint)
		}
		from[kind+" "+n] = o.EntryPoint
		into[n] = chunk(out)
	}
	return a, nil
}

// For lists the chunks to preload on a page whose script is the entry named
// entry and whose islands are spec's: the entry's imports first, then each
// island that mounts at once (Load "" or "eager") followed by its imports, in
// spec order and without repeats. The entry file itself is left out, since the
// page's <script> loads it. Islands that wait (idle, visible, media) are left
// out too, since preloading them would download what the page means to put
// off.
//
// An entry or island Assets doesn't know adds nothing (an island bundled with
// the entry, say), and a nil Assets gives nothing, so a page can call it
// whether or not the app loaded a manifest.
func (a *Assets) For(entry string, spec Spec) []string {
	if a == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(u string) {
		if !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	for _, u := range a.Entries[entry].Imports {
		add(u)
	}
	for _, is := range spec.Islands {
		if is.Load != "" && is.Load != "eager" {
			continue
		}
		ia, ok := a.Islands[is.Name]
		if !ok {
			continue
		}
		add(ia.File)
		for _, u := range ia.Imports {
			add(u)
		}
	}
	return out
}

// Names lists the islands Assets has a chunk for, sorted: the registry's lazy
// entries, as CheckIslands wants them. Islands bundled with the entry aren't
// among them. A nil Assets has none.
func (a *Assets) Names() []string {
	if a == nil {
		return nil
	}
	names := make([]string, 0, len(a.Islands))
	for n := range a.Islands {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}
