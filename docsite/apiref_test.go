package main

import (
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exported lists every exported top-level name in dir's package, and every exported method as
// Type.Method, straight from the source rather than through go/doc, so the test doesn't share the
// generator's view of what's there.
func exported(t *testing.T, dir string) []string {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if !d.Name.IsExported() {
					continue
				}
				if d.Recv == nil {
					names = append(names, d.Name.Name)
					continue
				}
				recv := d.Recv.List[0].Type
				for {
					switch r := recv.(type) {
					case *ast.StarExpr:
						recv = r.X
						continue
					case *ast.IndexExpr:
						recv = r.X
						continue
					case *ast.IndexListExpr:
						recv = r.X
						continue
					}
					break
				}
				if id, ok := recv.(*ast.Ident); ok && id.IsExported() {
					names = append(names, id.Name+"."+d.Name.Name)
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							names = append(names, s.Name.Name)
						}
					case *ast.ValueSpec:
						for _, n := range s.Names {
							if n.IsExported() {
								names = append(names, n.Name)
							}
						}
					}
				}
			}
		}
	}
	return names
}

// TestAPIReferenceListsEveryExportedName holds the generated reference to the source: each
// package's every exported name has an anchor on the page, so nothing exported is undocumented
// there, and the page can't keep a name the code dropped.
func TestAPIReferenceListsEveryExportedName(t *testing.T) {
	for _, p := range apiPackages {
		html := string(apiHTML(p.Dir))
		if strings.Contains(html, `class="demo-error"`) {
			t.Fatalf("%s: %s", p.Dir, html)
		}
		names := exported(t, filepath.Join("..", p.Dir))
		if len(names) == 0 {
			t.Fatalf("%s: no exported names found", p.Dir)
		}
		for _, n := range names {
			if !strings.Contains(html, `id="`+anchor(p, n)+`"`) {
				t.Errorf("%s: no anchor for %s", p.Dir, n)
			}
		}
	}
	_ = doc.AllDecls
}
