package goapplib

import (
	"bytes"
	"strings"
	"testing"
)

// renderComponent renders one of goapplib's component templates with only goapplib's own template
// functions, as an app that calls SetupTemplates would.
func renderComponent(t *testing.T, file, name string, data any) string {
	t.Helper()
	templates := SetupTemplates("./templates")
	loaded, err := templates.Loader.Load(file, "")
	if err != nil {
		t.Fatalf("load %s: %v", file, err)
	}
	var buf bytes.Buffer
	if err := templates.RenderHtmlTemplate(&buf, loaded[0], name, data, nil); err != nil {
		t.Fatalf("render %s: %v", name, err)
	}
	return buf.String()
}

func TestPaginationRendersWithGoapplibsFuncs(t *testing.T) {
	// Pagination reads the page's Query and Sort too, so it takes a page with both mixins.
	p := &struct {
		WithPagination
		WithFiltering
	}{WithPagination: WithPagination{CurrentPage: 1, PageSize: 10}}
	p.SetTotal(45, true)
	html := renderComponent(t, "components/Pagination.html", "Pagination", p)
	if !strings.Contains(html, `Showing page <span class="font-medium">2</span>`) || !strings.Contains(html, `of <span class="font-medium">5</span>`) {
		t.Errorf("page 1 (0-based) should show as page 2:\n%s", html)
	}
}

func TestSearchFilterRendersWithGoapplibsFuncs(t *testing.T) {
	data := map[string]any{"Query": "go", "ViewModes": []map[string]any{{"Value": "grid", "Label": "Grid"}, {"Value": "table", "Label": "Table"}}}
	html := renderComponent(t, "components/SearchFilter.html", "SearchFilter", data)
	if !strings.Contains(html, `value="go"`) || !strings.Contains(html, "rounded-r-md") {
		t.Errorf("SearchFilter didn't render its query and view modes:\n%s", html)
	}
}
