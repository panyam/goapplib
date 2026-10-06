package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/printer"
	"go/token"
	"html"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// apiPackage is a Go package the API reference documents, from its source in this checkout.
type apiPackage struct {
	Dir        string // relative to the repo root: "" for goapplib itself
	ImportPath string
	Slug       string // the anchor prefix: "goapplib-LoadAll"
}

var apiPackages = []apiPackage{
	{"", "github.com/panyam/goapplib", "goapplib"},
	{"page", "github.com/panyam/goapplib/page", "page"},
	{"wasmhost", "github.com/panyam/goapplib/wasmhost", "wasmhost"},
}

// apiGuides links a symbol to the guide page that teaches it, which its entry in the reference
// points to.
var apiGuides = map[string]string{
	"goapplib.View":                 "/guide/views/",
	"goapplib.Loader":               "/guide/concepts/#loaders",
	"goapplib.LoaderFunc":           "/guide/concepts/#loaders",
	"goapplib.LoadAll":              "/guide/concepts/#loaders",
	"goapplib.App":                  "/guide/concepts/#the-app-and-its-context",
	"goapplib.NewApp":               "/guide/concepts/#the-app-and-its-context",
	"goapplib.SetupTemplates":       "/guide/getting-started/#install",
	"goapplib.DefaultFuncMap":       "/guide/templates/#template-functions",
	"goapplib.Register":             "/guide/routing/#register",
	"goapplib.RegisterGroup":        "/guide/routing/#groups",
	"goapplib.PageGroup":            "/guide/routing/#groups",
	"goapplib.MuxBuilder":           "/guide/routing/#muxbuilder",
	"goapplib.WithTemplate":         "/guide/routing/#register",
	"goapplib.WithMiddleware":       "/guide/routing/#register",
	"goapplib.WithFragmentTemplate": "/guide/htmx/#one-route-a-page-or-a-fragment",
	"goapplib.SmartRegister":        "/guide/htmx/#one-route-a-page-or-a-fragment",
	"goapplib.HtmxResponse":         "/guide/htmx/#answering-an-action",
	"goapplib.BasePage":             "/guide/views/#goapplibs-mixins",
	"goapplib.WithPagination":       "/guide/views/#goapplibs-mixins",
	"goapplib.WithFiltering":        "/guide/views/#goapplibs-mixins",
	"goapplib.WithAuth":             "/guide/views/#goapplibs-mixins",
	"goapplib.WithHtmx":             "/guide/htmx/#one-route-a-page-or-a-fragment",
	"goapplib.AuthProvider":         "/guide/views/#goapplibs-mixins",
	"goapplib.EntityListingData":    "/guide/templates/#components",
	"page.Spec":                     "/guide/islands/#the-go-side",
	"page.Island":                   "/guide/islands/#when-each-island-mounts",
	"page.LoadEsbuildMetafile":      "/guide/islands/#lazy-islands-and-preload-links",
	"page.Assets":                   "/guide/islands/#lazy-islands-and-preload-links",
	"page.CheckIslands":             "/guide/islands/#checking-and-debugging",
	"wasmhost.Serve":                "/guide/wasmhost/#the-pieces",
	"wasmhost.ServeRebuild":         "/guide/wasmhost/#fixed-handlers-and-rebuilt-ones",
	"wasmhost.Host":                 "/guide/wasmhost/#testing-it-natively",
	"wasmhost.Cache":                "/guide/wasmhost-state/#a-cache-for-state-the-service-can-rebuild",
	"wasmhost.BrowserCache":         "/guide/wasmhost-state/#a-cache-for-state-the-service-can-rebuild",
	"wasmhost.CacheKey":             "/guide/wasmhost-state/#a-cache-for-state-the-service-can-rebuild",
}

func anchor(p apiPackage, name string) string {
	return p.Slug + "-" + strings.ReplaceAll(name, ".", "-")
}

// apiHTML is the `apiref` template function: {{ apiref "page" }} renders the exported API of the
// package in that directory of this checkout (types, functions, methods, constants and variables)
// with each declaration and its doc comment, read with go/doc when the site is built, so the
// reference can't drift from the code. Declarations from a file with a build constraint
// (wasmhost's js && wasm files) say so. A package that doesn't parse fails the build.
func apiHTML(dir string) template.HTML {
	var p apiPackage
	for _, q := range apiPackages {
		if q.Dir == dir {
			p = q
		}
	}
	if p.Slug == "" {
		return apiError(dir, fmt.Errorf("not one of apiPackages"))
	}
	out, err := renderAPI(p)
	if err != nil {
		return apiError(dir, err)
	}
	return template.HTML(out)
}

func apiError(dir string, err error) template.HTML {
	failDemo("apiref "+dir, err)
	return template.HTML(`<p class="demo-error">` + html.EscapeString(err.Error()) + `</p>`)
}

var buildLine = regexp.MustCompile(`(?m)^//go:build (.+)$`)

func renderAPI(p apiPackage) (string, error) {
	src := filepath.Join(projectRoot, "..", p.Dir)
	entries, err := os.ReadDir(src)
	if err != nil {
		return "", err
	}
	fset := token.NewFileSet()
	var files []*ast.File
	constraint := map[string]string{} // file -> its //go:build expression
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		path := filepath.Join(src, n)
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		f, err := parser.ParseFile(fset, path, data, parser.ParseComments)
		if err != nil {
			return "", err
		}
		files = append(files, f)
		if m := buildLine.FindSubmatch(data); m != nil {
			constraint[path] = string(m[1])
		}
	}
	pkg, err := doc.NewFromFiles(fset, files, p.ImportPath)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	decl := func(node any) string {
		var buf bytes.Buffer
		cfg := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 4}
		_ = cfg.Fprint(&buf, fset, node)
		return html.EscapeString(buf.String())
	}
	note := func(pos token.Pos) string {
		if c, ok := constraint[fset.Position(pos).Filename]; ok {
			return ` <span class="apiref-tag">build: ` + html.EscapeString(c) + `</span>`
		}
		return ""
	}
	guide := func(name string) string {
		if g, ok := apiGuides[p.Slug+"."+name]; ok {
			return `<p class="apiref-guide">Guide: <a href="` + PathPrefix + g + `">` + PathPrefix + g + `</a></p>`
		}
		return ""
	}
	value := func(v *doc.Value, level string) {
		for _, n := range v.Names {
			if ast.IsExported(n) {
				w(`<span id="%s"></span>`, anchor(p, n))
			}
		}
		w(`<pre><code class="language-go">%s</code></pre>%s`, decl(v.Decl), string(pkg.HTML(v.Doc)))
		_ = level
	}
	fn := func(f *doc.Func, name, level string) {
		w(`<%s id="%s"><code>%s</code>%s</%s>`, level, anchor(p, name), html.EscapeString(name), note(f.Decl.Pos()), level)
		w(`<pre><code class="language-go">%s</code></pre>%s%s`, decl(f.Decl), string(pkg.HTML(f.Doc)), guide(name))
	}

	w(`<section class="apiref"><h2 id="%s">%s</h2><p><code>import "%s"</code></p>%s`, p.Slug, p.Slug, p.ImportPath, string(pkg.HTML(pkg.Doc)))
	if len(pkg.Consts)+len(pkg.Vars) > 0 {
		w(`<h3 id="%s-values">Constants and variables</h3>`, p.Slug)
		for _, v := range pkg.Consts {
			value(v, "h3")
		}
		for _, v := range pkg.Vars {
			value(v, "h3")
		}
	}
	for _, f := range pkg.Funcs {
		fn(f, f.Name, "h3")
	}
	types := pkg.Types
	sort.SliceStable(types, func(i, j int) bool { return types[i].Name < types[j].Name })
	for _, t := range types {
		w(`<h3 id="%s"><code>%s</code>%s</h3>`, anchor(p, t.Name), t.Name, note(t.Decl.Pos()))
		w(`<pre><code class="language-go">%s</code></pre>%s%s`, decl(t.Decl), string(pkg.HTML(t.Doc)), guide(t.Name))
		for _, v := range t.Consts {
			value(v, "h4")
		}
		for _, v := range t.Vars {
			value(v, "h4")
		}
		for _, f := range t.Funcs {
			fn(f, f.Name, "h4")
		}
		for _, m := range t.Methods {
			fn(m, t.Name+"."+m.Name, "h4")
		}
	}
	w(`</section>`)
	return oneBlock(b.String()), nil
}

var preBlock = regexp.MustCompile(`(?s)<pre>.*?</pre>`)

// oneBlock makes html safe to drop into markdown as a single raw-HTML block: markdown ends an HTML
// block at its first blank line, so newlines inside <pre> become &#10; and the rest become spaces.
func oneBlock(s string) string {
	s = preBlock.ReplaceAllStringFunc(s, func(pre string) string { return strings.ReplaceAll(pre, "\n", "&#10;") })
	return strings.ReplaceAll(s, "\n", " ")
}
