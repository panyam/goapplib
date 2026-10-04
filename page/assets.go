package page

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
)

// Assets says which built files each island needs, so a page can write
// <link rel="modulepreload"> for the islands its spec mounts at once (see
// For and the IslandPreloads partial in templates/page/Islands.html). It's
// the browser side's lazy registry entries (tsappkit's lazy) seen from Go:
// each lazy island is its own chunk, and without preload links the browser
// only finds that chunk, then its imports, after the entry has run.
//
// The JSON form is the shape any bundler's build can write:
//
//	{"islands": {"hero": {"file": "/static/chunks/hero-X.js", "imports": ["/static/chunks/chunk-Y.js"]}}}
//
// LoadAssets reads that; LoadEsbuildMetafile builds it from esbuild's metafile.
type Assets struct {
	Islands map[string]IslandAsset `json:"islands"`
}

// IslandAsset is one island's chunk. File is the URL of the chunk holding the
// island's module; Imports are the URLs of every chunk it statically imports,
// directly or through another chunk, in a stable order. Neither includes the
// page's entry script, which the page loads itself.
type IslandAsset struct {
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
// island names and URLs.
type EsbuildOptions struct {
	// OutDir is the output directory as the metafile writes it (the --outdir
	// relative to where esbuild ran, "dist" say). It's cut from the front of
	// each output path before URLPrefix is added.
	OutDir string
	// URLPrefix is where the page serves OutDir, such as "/static/".
	URLPrefix string
	// Name gives the island name for the source file of a dynamically
	// imported chunk (its path as the metafile has it, "web/islands/hero.ts").
	// The default is the file's base name without its extension ("hero"), so
	// a registry entry `hero: lazy(() => import("./islands/hero"))` needs no
	// setting. Returning "" leaves that chunk out.
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
// with --splitting --format=esm --metafile. Each output some other output
// reaches through a dynamic import() is an island chunk, named by
// opts.Name from its source file; its Imports are the chunks it reaches
// through static imports. Two chunks with the same island name are an error,
// since the page couldn't tell which one the spec means.
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

	dynamic := map[string]bool{}
	for _, o := range m.Outputs {
		for _, imp := range o.Imports {
			if imp.Kind == "dynamic-import" {
				dynamic[imp.Path] = true
			}
		}
	}
	a := &Assets{Islands: map[string]IslandAsset{}}
	from := map[string]string{}
	for out, o := range m.Outputs {
		if !dynamic[out] || o.EntryPoint == "" {
			continue
		}
		n := name(o.EntryPoint)
		if n == "" {
			continue
		}
		if prev, ok := from[n]; ok {
			return nil, fmt.Errorf("%s: island %q has two chunks, from %s and %s", file, n, prev, o.EntryPoint)
		}
		from[n] = o.EntryPoint

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
		a.Islands[n] = IslandAsset{File: url(out), Imports: imports}
	}
	return a, nil
}

// For lists the chunks to preload for spec: those of each island that mounts
// at once (Load "" or "eager"), each followed by its imports, in spec order
// and without repeats. Islands that wait (idle, visible, media) are left out,
// since preloading them would download what the page means to put off. An
// island Assets doesn't know is skipped (it's bundled with the entry, or
// built some other way), and a nil Assets gives nothing, so a page can call
// it whether or not the app loaded a manifest.
func (a *Assets) For(spec Spec) []string {
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
