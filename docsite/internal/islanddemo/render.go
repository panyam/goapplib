// Package islanddemo renders an island demo's page at build time, the way an app's server renders
// one per request: a page.Spec, checked, written by goapplib's PageSpecScript partial, with
// IslandPreloads links from esbuild's metafile. GitHub Pages can't run a Go server, so build.mjs
// runs each island demo's page/ main once and serves what it wrote.
package islanddemo

import (
	"flag"
	"html/template"
	"log"
	"os"

	"github.com/panyam/goapplib/page"
)

// Main is the whole main of a demo's page/ program. tmpl defines "page", and gets the spec as
// .Spec, the preload URLs as .Preloads and the entry script's URL as .Main. It reads the flags
// build.mjs passes and writes the page to stdout, and exits non-zero on a spec that doesn't
// validate, an unreadable metafile or a template error, which fails the build.
func Main(spec page.Spec, tmpl string) {
	meta := flag.String("meta", "", "esbuild's metafile for the demo's bundle")
	outdir := flag.String("outdir", "", "the bundle's --outdir, as the metafile writes it")
	partial := flag.String("partial", "../templates/page/Islands.html", "goapplib's PageSpecScript partial")
	mainURL := flag.String("main", "main.js", "the entry script's URL")
	flag.Parse()

	if err := spec.Validate(); err != nil {
		log.Fatal(err)
	}
	assets, err := page.LoadEsbuildMetafile(*meta, page.EsbuildOptions{OutDir: *outdir, URLPrefix: "./"})
	if err != nil {
		log.Fatal(err)
	}
	t, err := template.ParseFiles(*partial)
	if err != nil {
		log.Fatal(err)
	}
	if t, err = t.New("demo").Parse(tmpl); err != nil {
		log.Fatal(err)
	}
	data := map[string]any{"Spec": spec, "Preloads": assets.For("main", spec), "Main": *mainURL}
	if err := t.ExecuteTemplate(os.Stdout, "page", data); err != nil {
		log.Fatal(err)
	}
}
