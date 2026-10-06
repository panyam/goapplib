// The islands-solid demo's page, rendered at build time by internal/islanddemo: one island, a
// SolidIsland, over a placeholder Go draws in its slot.
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
		{Name: "counter", Slot: "main"},
	}}, tmpl)
}
