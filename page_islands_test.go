package goapplib

import (
	"bytes"
	"encoding/json"
	"html/template"
	"regexp"
	"testing"

	"github.com/panyam/goapplib/page"
)

// withThings is an app's own spec, extending page.Spec as the page package
// shows, to check the partial writes it whole.
type withThings struct {
	page.Spec
	Things []string `json:"things"`
}

func (s withThings) JSON() template.JS {
	c := s
	c.Spec = s.Spec.Normalized()
	return page.ScriptJSON(c)
}

func renderSpecScript(t *testing.T, spec any) string {
	t.Helper()
	templates := SetupTemplates("./templates")
	loaded, err := templates.Loader.Load("page/Islands.html", "")
	if err != nil || len(loaded) == 0 {
		t.Fatalf("loading page/Islands.html: %v", err)
	}
	var buf bytes.Buffer
	if err := templates.RenderHtmlTemplate(&buf, loaded[0], "PageSpecScript", spec, nil); err != nil {
		t.Fatalf("rendering PageSpecScript: %v", err)
	}
	return buf.String()
}

var specScript = regexp.MustCompile(`^<script type="application/json" id="page-spec">(.*)</script>$`)

func TestPageSpecScript(t *testing.T) {
	for name, spec := range map[string]any{
		"page.Spec":    page.Spec{Layout: "main", Islands: []page.Island{{Name: "player", Slot: "main", Config: map[string]any{"x": "</script>"}}}},
		"extended one": withThings{Spec: page.Spec{Layout: "main"}, Things: []string{"dice"}},
	} {
		out := renderSpecScript(t, spec)
		m := specScript.FindStringSubmatch(out)
		if m == nil {
			t.Fatalf("%s: not one spec script: %s", name, out)
		}
		var got map[string]any
		if err := json.Unmarshal([]byte(m[1]), &got); err != nil {
			t.Fatalf("%s: the script's JSON doesn't parse (was it escaped twice?): %v\n%s", name, err, m[1])
		}
		if got["layout"] != "main" {
			t.Errorf("%s: layout = %#v", name, got["layout"])
		}
	}
}
