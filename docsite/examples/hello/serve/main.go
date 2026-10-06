// Command serve runs the hello app:
//
//	go run ./serve -goapplib-templates "$(go list -m -f '{{.Dir}}' github.com/panyam/goapplib)/templates"
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/panyam/goapplib/docsite/examples/hello"
)

func main() {
	addr := flag.String("addr", ":8080", "address to serve on")
	goapplibTemplates := flag.String("goapplib-templates", "", "goapplib's templates/ folder")
	flag.Parse()

	site := &hello.Site{Name: "Hello", Tagline: "A goapplib app."}
	log.Printf("serving on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, hello.NewHandler(site, "templates", *goapplibTemplates)))
}
