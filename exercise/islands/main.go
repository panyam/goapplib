// The exercise for mission #60 (goapplib issue 61): a server-rendered page whose spec names three
// islands with different load strategies, drawn by goapplib's own PageSpecScript partial. run.mjs
// drives it in headless Chromium.
//
//	go run ./exercise/islands -dist exercise/islands/dist
//
// It prints the address it listens on, so a driver can start it on port 0.
package main

import (
	"embed"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"path/filepath"

	"github.com/panyam/goapplib/page"
)

//go:embed templates/page.html
var pageTemplate embed.FS

var spec = page.Spec{Layout: "exercise", Islands: []page.Island{
	{Name: "hero", Slot: "top"},
	{Name: "below", Slot: "bottom", Load: "visible"},
	{Name: "narrow", Slot: "side", Load: "media:(max-width: 600px)"},
}}

// newHandler serves the page at / and the built bundle from dist at /static/. partial is goapplib's
// templates/page/Islands.html, which defines PageSpecScript and IslandPreloads. The page preloads
// its eager islands' chunks, read from esbuild's metafile in dist (paths in it start "dist/", as
// esbuild runs from exercise/islands).
func newHandler(dist, partial string) (http.Handler, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}
	assets, err := page.LoadEsbuildMetafile(filepath.Join(dist, "meta.json"), page.EsbuildOptions{OutDir: "dist", URLPrefix: "/static/"})
	if err != nil {
		return nil, err
	}
	t, err := template.New("").ParseFiles(partial)
	if err != nil {
		return nil, err
	}
	if t, err = t.ParseFS(pageTemplate, "templates/page.html"); err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(dist))))
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := t.ExecuteTemplate(w, "page", map[string]any{"Spec": spec, "Preloads": assets.For(spec)}); err != nil {
			log.Printf("render: %v", err)
		}
	})
	return mux, nil
}

func main() {
	addr := flag.String("addr", "127.0.0.1:0", "address to listen on")
	dist := flag.String("dist", "exercise/islands/dist", "the built bundle")
	partial := flag.String("partial", "templates/page/Islands.html", "goapplib's PageSpecScript partial")
	flag.Parse()
	h, err := newHandler(*dist, *partial)
	if err != nil {
		log.Fatal(err)
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("listening on http://%s/\n", ln.Addr())
	log.Fatal(http.Serve(ln, h))
}
