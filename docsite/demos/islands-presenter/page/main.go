// The islands-presenter demo's page, rendered at build time by internal/islanddemo: two islands
// driven by one presenter, the second loading only once it's scrolled into view.
package main

import (
	_ "embed"

	"github.com/panyam/goapplib/docsite/internal/islanddemo"
	"github.com/panyam/goapplib/page"
)

//go:embed page.html
var tmpl string

func main() {
	islanddemo.Main(page.Spec{Layout: "demo", Islands: []page.Island{
		{Name: "tools", Slot: "tools"},
		{Name: "history", Slot: "history", Load: "visible"},
	}}, tmpl)
}
