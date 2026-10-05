package main

import (
	"fmt"
	"html"
	"html/template"
	"log"
	"os"
	"sync"
)

// demoFailures records every demo a page asked for that has no source, so main fails the build. A
// template function's error alone would only blank the page, and s3gen would still exit 0.
var (
	demoFailuresMu sync.Mutex
	demoFailures   []string
)

func failDemo(name string, err error) {
	demoFailuresMu.Lock()
	defer demoFailuresMu.Unlock()
	demoFailures = append(demoFailures, fmt.Sprintf("%s: %v", name, err))
}

// defaultDemoHeight is the iframe's height in CSS pixels when a page doesn't give one.
const defaultDemoHeight = 240

// demoHTML is the `demo` template function: {{ demo "hello" }}, or {{ demo "hello" 320 }} for a
// taller frame, embeds the live demo whose source is demos/hello/ (an index.html and a main.ts that
// build.mjs bundles into static/demos/hello/). Each demo runs in its own iframe, so its scripts,
// workers and wasm can't reach the page or another demo. A demo marks itself done with ready() or
// failed() from demos/_lib/frame.ts, which set data-demo on its <html>; run.mjs waits for that.
func demoHTML(name string, height ...int) template.HTML {
	for _, f := range []string{"index.html", "main.ts"} {
		if _, err := os.Stat("demos/" + name + "/" + f); err != nil {
			failDemo(name, fmt.Errorf("no demos/%s/%s", name, f))
			return template.HTML(`<p class="demo-error">No demo named ` + html.EscapeString(name) + `.</p>`)
		}
	}
	h := defaultDemoHeight
	if len(height) > 0 {
		h = height[0]
	}
	src := assetURL("static/demos/" + name + "/index.html")
	source := "https://github.com/panyam/goapplib/tree/main/docsite/demos/" + name
	// One raw-HTML block with no blank line in it, because markdown ends an HTML block at the first
	// blank line.
	return template.HTML(fmt.Sprintf(
		`<figure class="demo" data-demo="%[1]s"><iframe src="%[2]s" title="Live demo: %[1]s" style="height:%[3]dpx"></iframe>`+
			`<figcaption>Live demo, running in your browser.<a href="%[2]s" target="_blank" rel="noopener">Open it on its own</a><a href="%[4]s" target="_blank" rel="noopener">Source</a></figcaption></figure>`,
		html.EscapeString(name), html.EscapeString(src), h, source))
}

// exitOnDemoFailures ends a build that rendered a page asking for a demo with no source.
func exitOnDemoFailures() {
	demoFailuresMu.Lock()
	defer demoFailuresMu.Unlock()
	if len(demoFailures) == 0 {
		return
	}
	for _, f := range demoFailures {
		log.Printf("demo failed: %s", f)
	}
	os.Exit(1)
}
