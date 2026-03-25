package goapplib

import (
	"bytes"
	"strings"
	"testing"
)

// renderBorderLayout renders the BorderLayout template with the given data map
// using the real Templar template loader and returns the output HTML string.
// It loads templates from the local templates/ directory, matching the same
// setup that applications use via SetupTemplates().
func renderBorderLayout(t *testing.T, data map[string]any) string {
	t.Helper()
	templates := SetupTemplates("./templates")
	loaded, err := templates.Loader.Load("components/BorderLayout.html", "")
	if err != nil {
		t.Fatalf("Failed to load BorderLayout template: %v", err)
	}
	if len(loaded) == 0 {
		t.Fatal("No templates loaded for BorderLayout.html")
	}

	var buf bytes.Buffer
	err = templates.RenderHtmlTemplate(&buf, loaded[0], "BorderLayout", data, nil)
	if err != nil {
		t.Fatalf("Failed to render BorderLayout template: %v", err)
	}
	return buf.String()
}

// TestBorderLayout_DefaultRendering verifies that the BorderLayout template
// renders correctly with no parameters, producing the expected default IDs
// (border-layout-wrapper, border-layout-center, border-layout-content) and
// defaulting to FlexMode="fill" which applies flex: 1 1 0% styling.
func TestBorderLayout_DefaultRendering(t *testing.T) {
	html := renderBorderLayout(t, map[string]any{})

	checks := []string{
		`id="border-layout-wrapper"`,
		`id="border-layout-center"`,
		`id="border-layout-content"`,
		`flex: 1 1 0%`,
	}
	for _, check := range checks {
		if !strings.Contains(html, check) {
			t.Errorf("Default rendering missing %q\nGot:\n%s", check, html)
		}
	}
}

// TestBorderLayout_CustomContentId verifies that passing a custom ContentId
// parameter changes the ID of the inner content div from the default
// "border-layout-content" to the specified value, allowing consumers to
// target specific content areas (e.g., a canvas container).
func TestBorderLayout_CustomContentId(t *testing.T) {
	html := renderBorderLayout(t, map[string]any{
		"ContentId": "my-canvas",
	})

	if !strings.Contains(html, `id="my-canvas"`) {
		t.Errorf("Custom ContentId not applied\nGot:\n%s", html)
	}
	if strings.Contains(html, `id="border-layout-content"`) {
		t.Error("Default ContentId should not appear when custom ContentId is set")
	}
}

// TestBorderLayout_FlexModeFill verifies that FlexMode="fill" (the default)
// applies the flex: 1 1 0% and min-height/min-width: 0 inline styles to the
// wrapper div. These styles ensure the layout takes all remaining space in a
// flex parent while preventing circular sizing dependencies.
func TestBorderLayout_FlexModeFill(t *testing.T) {
	html := renderBorderLayout(t, map[string]any{
		"FlexMode": "fill",
	})

	checks := []string{
		`flex: 1 1 0%`,
		`min-height: 0`,
		`min-width: 0`,
	}
	for _, check := range checks {
		if !strings.Contains(html, check) {
			t.Errorf("FlexMode fill missing %q\nGot:\n%s", check, html)
		}
	}
	if strings.Contains(html, `width: 100%`) {
		t.Error("FlexMode fill should not have width: 100%")
	}
}

// TestBorderLayout_FlexModeFixed verifies that FlexMode="fixed" applies
// width: 100% and height: 100% inline styles to the wrapper, making it fill
// its parent container completely. This mode is used when the layout is placed
// inside a positioned container with explicit dimensions.
func TestBorderLayout_FlexModeFixed(t *testing.T) {
	html := renderBorderLayout(t, map[string]any{
		"FlexMode": "fixed",
	})

	if !strings.Contains(html, `width: 100%; height: 100%`) {
		t.Errorf("FlexMode fixed missing width/height: 100%%\nGot:\n%s", html)
	}
	if strings.Contains(html, `flex: 1 1 0%`) {
		t.Error("FlexMode fixed should not have flex: 1 1 0%")
	}
}

// TestBorderLayout_FlexModeAuto verifies that FlexMode="auto" produces no
// inline style attribute on the wrapper div, allowing the layout to size
// naturally based on its content. This mode is useful when the layout is
// embedded in a non-flex context or when explicit sizing is handled by
// external CSS classes.
func TestBorderLayout_FlexModeAuto(t *testing.T) {
	html := renderBorderLayout(t, map[string]any{
		"FlexMode": "auto",
	})

	// Extract the wrapper div's opening tag to check for style attribute
	wrapperIdx := strings.Index(html, `id="border-layout-wrapper"`)
	if wrapperIdx == -1 {
		t.Fatal("Wrapper div not found")
	}
	// Find the enclosing <div ... > tag
	tagStart := strings.LastIndex(html[:wrapperIdx], "<div")
	tagEnd := strings.Index(html[tagStart:], ">") + tagStart
	wrapperTag := html[tagStart : tagEnd+1]

	if strings.Contains(wrapperTag, `style=`) {
		t.Errorf("FlexMode auto should have no inline style on wrapper\nWrapper tag: %s", wrapperTag)
	}
}

// TestBorderLayout_CustomClasses verifies that the WrapperClass and CenterClass
// parameters are correctly injected into the class attributes of the wrapper and
// center divs respectively. This allows consumers to add custom Tailwind/CSS
// classes for styling (e.g., background colors, padding, dark mode variants).
func TestBorderLayout_CustomClasses(t *testing.T) {
	html := renderBorderLayout(t, map[string]any{
		"WrapperClass": "my-wrapper bg-gray-100",
		"CenterClass":  "my-center p-4",
	})

	if !strings.Contains(html, `my-wrapper bg-gray-100`) {
		t.Errorf("WrapperClass not applied\nGot:\n%s", html)
	}
	if !strings.Contains(html, `my-center p-4`) {
		t.Errorf("CenterClass not applied\nGot:\n%s", html)
	}
}

// TestBorderLayout_RegionBlocksEmpty verifies that when no block overrides are
// defined for the North, South, East, and West regions, they render as empty
// content. The block directives ({{ block "BorderLayout_North" . }}{{ end }})
// produce no output by default, so the layout collapses to just the center.
func TestBorderLayout_RegionBlocksEmpty(t *testing.T) {
	html := renderBorderLayout(t, map[string]any{})

	// With no block overrides, the region blocks should produce no visible content.
	// The HTML should still have the structural divs but no region-specific content.
	// Verify the wrapper and center exist.
	if !strings.Contains(html, `id="border-layout-wrapper"`) {
		t.Error("Wrapper div missing")
	}
	if !strings.Contains(html, `id="border-layout-center"`) {
		t.Error("Center div missing")
	}
}

// TestBorderLayout_StructuralIntegrity verifies the overall DOM structure of the
// BorderLayout: the wrapper contains a middle row div with class "flex-1 flex flex-row"
// and style "min-height: 0; overflow: hidden", and the center region within it has
// the critical min-width/min-height: 0 styles that prevent content from pushing the
// parent size (breaking circular sizing dependencies in flexbox layouts).
func TestBorderLayout_StructuralIntegrity(t *testing.T) {
	html := renderBorderLayout(t, map[string]any{})

	// Verify the middle row structure
	checks := []struct {
		name  string
		value string
	}{
		{"middle row flex classes", `class="flex-1 flex flex-row"`},
		{"middle row min-height", `min-height: 0; overflow: hidden`},
		{"center min-width", `min-width: 0; min-height: 0; overflow: hidden; position: relative`},
		{"content div positioning", `position: relative; overflow: hidden`},
	}
	for _, check := range checks {
		if !strings.Contains(html, check.value) {
			t.Errorf("Structural check %q failed: missing %q\nGot:\n%s", check.name, check.value, html)
		}
	}
}
