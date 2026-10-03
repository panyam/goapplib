package page

import (
	"encoding/json"
	"html/template"
	"os"
	"strings"
	"testing"
)

func TestJSONIsSafeInsideAScriptTag(t *testing.T) {
	s := Spec{Layout: "main", Islands: []Island{
		{Name: "player", Slot: "main", Config: map[string]any{"note": "</script><script>alert(1)</script> <!-- x"}},
	}}
	out := string(s.JSON())
	for _, bad := range []string{"</script", "<!--"} {
		if strings.Contains(strings.ToLower(out), bad) {
			t.Fatalf("JSON() contains %q: %s", bad, out)
		}
	}
	var back Spec
	if err := json.Unmarshal([]byte(out), &back); err != nil {
		t.Fatalf("JSON() doesn't parse back: %v\n%s", err, out)
	}
	if back.Islands[0].Config["note"] != s.Islands[0].Config["note"] {
		t.Fatalf("config changed on the way through: %q", back.Islands[0].Config["note"])
	}
}

func TestJSONShape(t *testing.T) {
	s := Spec{Layout: "side", Islands: []Island{{Name: "chat", Slot: "side", Presentation: "panel"}}}
	var got map[string]any
	if err := json.Unmarshal([]byte(s.JSON()), &got); err != nil {
		t.Fatal(err)
	}
	island := got["islands"].([]any)[0].(map[string]any)
	want := map[string]any{"name": "chat", "slot": "side", "presentation": "panel"}
	for k, v := range want {
		if island[k] != v {
			t.Fatalf("%s = %#v, want %#v", k, island[k], v)
		}
	}
	if c, ok := island["config"].(map[string]any); !ok || len(c) != 0 {
		t.Fatalf("a nil config = %#v, want {}", island["config"])
	}
	if got["layout"] != "side" {
		t.Fatalf("layout = %#v", got["layout"])
	}
	// No islands is an empty list, not null, so the browser can always loop.
	if out := string(Spec{Layout: "main"}.JSON()); !strings.Contains(out, `"islands":[]`) {
		t.Fatalf("JSON() = %s, want an empty islands list", out)
	}
}

func TestNormalizedLeavesTheSpecAlone(t *testing.T) {
	s := Spec{Layout: "main", Islands: []Island{{Name: "player", Slot: "main"}}}
	n := s.Normalized()
	if n.Islands[0].Config == nil {
		t.Fatal("Normalized() left a nil config")
	}
	if s.Islands[0].Config != nil {
		t.Fatal("Normalized() changed the spec it was called on")
	}
}

func TestValidate(t *testing.T) {
	ok := Spec{Layout: "main", Islands: []Island{{Name: "player", Slot: "main"}, {Name: "chat", Slot: "side-2"}}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid spec: %v", err)
	}
	for name, s := range map[string]Spec{
		"no layout":   {Islands: []Island{{Name: "player", Slot: "main"}}},
		"no name":     {Layout: "main", Islands: []Island{{Slot: "main"}}},
		"no slot":     {Layout: "main", Islands: []Island{{Name: "player"}}},
		"shared slot": {Layout: "main", Islands: []Island{{Name: "player", Slot: "main"}, {Name: "chat", Slot: "main"}}},
		"bad slot":    {Layout: "main", Islands: []Island{{Name: "player", Slot: `main"]`}}},
		"capitals":    {Layout: "main", Islands: []Island{{Name: "player", Slot: "Main"}}},
	} {
		if err := s.Validate(); err == nil {
			t.Errorf("%s: Validate() = nil, want an error", name)
		}
	}
}

func TestSlots(t *testing.T) {
	s := Spec{Layout: "main", Islands: []Island{{Name: "player", Slot: "main"}, {Name: "chat", Slot: "side"}}}
	if got := strings.Join(s.Slots(), ","); got != "main,side" {
		t.Fatalf("Slots() = %s", got)
	}
}

// An app's own spec, as the package doc shows: Spec embedded, plus a field
// of its own, written through Normalized and ScriptJSON.
type appSpec struct {
	Spec
	Things []map[string]any `json:"things"`
}

func (s appSpec) JSON() template.JS {
	c := s
	c.Spec = s.Spec.Normalized()
	if c.Things == nil {
		c.Things = []map[string]any{}
	}
	return ScriptJSON(c)
}

func TestExtendingTheSpec(t *testing.T) {
	s := appSpec{Spec: Spec{Layout: "main", Islands: []Island{{Name: "player", Slot: "main"}}}, Things: []map[string]any{{"kind": "dice"}}}
	var got map[string]any
	if err := json.Unmarshal([]byte(s.JSON()), &got); err != nil {
		t.Fatal(err)
	}
	// One flat object: the embedded spec's fields beside the app's own.
	if got["layout"] != "main" || len(got["islands"].([]any)) != 1 || got["things"].([]any)[0].(map[string]any)["kind"] != "dice" {
		t.Fatalf("extended spec = %#v", got)
	}
	if _, nested := got["Spec"]; nested {
		t.Fatal("the embedded Spec was written as a nested object")
	}
	if c, ok := got["islands"].([]any)[0].(map[string]any)["config"].(map[string]any); !ok || len(c) != 0 {
		t.Fatal("Normalized() didn't reach the embedded spec's islands")
	}
}

func TestScriptJSONOfSomethingUnencodable(t *testing.T) {
	if got := ScriptJSON(map[string]any{"f": func() {}}); got != "null" {
		t.Fatalf("ScriptJSON(func) = %s, want null", got)
	}
}

// testdata/spec.json is what tsappkit's readSpec is tested against
// (tsappkit/src/page/spec.test.ts), so a change to the format on either side
// fails a test.
func TestJSONMatchesTheSpecTsappkitReads(t *testing.T) {
	s := Spec{Layout: "drawer", Islands: []Island{
		{Name: "player", Slot: "main", Presentation: "page", Config: map[string]any{"note": "</script>", "urls": []any{"/a.json"}}},
		{Name: "chat", Slot: "side"},
	}}
	want, err := os.ReadFile("testdata/spec.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(s.JSON()); got != strings.TrimSpace(string(want)) {
		t.Fatalf("JSON() =\n%s\nwant (testdata/spec.json)\n%s", got, want)
	}
}
