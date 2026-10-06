// The islands-load demo's page, rendered at build time by internal/islanddemo: four islands, one
// per load strategy.
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
		{Name: "hero", Slot: "top"},
		{Name: "later", Slot: "middle", Load: "idle"},
		{Name: "below", Slot: "bottom", Load: "visible"},
		{Name: "narrow", Slot: "side", Load: "media:(max-width: 600px)"},
	}}, tmpl)
}
