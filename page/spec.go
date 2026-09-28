// Package page describes which islands a page mounts and where, as a Spec
// the server writes into the page for the browser to read (tsappkit's island
// page mounts each island from a registry by name).
//
// A layout template places the slots (elements with data-slot); the Spec
// says which island goes in each, how it's presented, and its config. The
// partial templates/page/Islands.html writes it as
// <script type="application/json" id="page-spec">.
//
// An app that needs more in its spec (the things a page starts with, say)
// embeds Spec in its own type and writes it with ScriptJSON:
//
//	type Spec struct {
//		page.Spec
//		Things []Thing `json:"things"`
//	}
//
//	func (s Spec) JSON() template.JS {
//		c := s
//		c.Spec = s.Spec.Normalized()
//		return page.ScriptJSON(c)
//	}
//
// encoding/json flattens the embedded Spec, so the browser sees one object
// with layout, islands and things.
package page

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"regexp"
)

// Spec is a page's layout and the islands it mounts, in mount order.
type Spec struct {
	// Layout names the arrangement of slots, for code that keeps state per
	// layout. The template that draws it is chosen by the page, not by this.
	Layout  string   `json:"layout"`
	Islands []Island `json:"islands"`
}

// Island is one island on the page. Name picks the factory from the
// browser's registry; Slot is the data-slot of the element it mounts in.
// Presentation says how the layout shows it (an app's own words: "page",
// "panel", "strip"), for islands that draw differently in each. Config is
// handed to the factory as is, so it must be JSON.
type Island struct {
	Name         string         `json:"name"`
	Slot         string         `json:"slot"`
	Presentation string         `json:"presentation,omitempty"`
	Config       map[string]any `json:"config"`
}

// Slot names are used in a CSS attribute selector, so keep them plain.
var slotName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Validate reports a spec the browser couldn't mount as meant: no layout, an
// island without a name or with a slot name that isn't lowercase letters,
// digits and dashes, or two islands in one slot. An app that extends the
// spec calls it from its own Validate and adds its own checks.
func (s Spec) Validate() error {
	if s.Layout == "" {
		return fmt.Errorf("page spec has no layout")
	}
	seen := map[string]string{}
	for i, is := range s.Islands {
		if is.Name == "" {
			return fmt.Errorf("island %d has no name", i)
		}
		if !slotName.MatchString(is.Slot) {
			return fmt.Errorf("island %q has slot %q; want lowercase letters, digits and dashes", is.Name, is.Slot)
		}
		if other, ok := seen[is.Slot]; ok {
			return fmt.Errorf("islands %q and %q share slot %q", other, is.Name, is.Slot)
		}
		seen[is.Slot] = is.Name
	}
	return nil
}

// Slots lists the slots the spec fills, in mount order.
func (s Spec) Slots() []string {
	out := make([]string, len(s.Islands))
	for i, is := range s.Islands {
		out[i] = is.Slot
	}
	return out
}

// Normalized is a copy of the spec with a nil island list as empty and each
// nil Config as an empty map, so the browser always gets a list and objects
// rather than null. The spec itself is left alone.
func (s Spec) Normalized() Spec {
	c := s
	c.Islands = make([]Island, len(s.Islands))
	for i, is := range s.Islands {
		if is.Config == nil {
			is.Config = map[string]any{}
		}
		c.Islands[i] = is
	}
	return c
}

// JSON is the spec for a <script type="application/json"> element, written
// through ScriptJSON after Normalized.
func (s Spec) JSON() template.JS {
	return ScriptJSON(s.Normalized())
}

// ScriptJSON is v as JSON that's safe inside a <script> element: every '<',
// '>' and '&' is escaped, so no string in it can close the script or open a
// comment. A value that can't be encoded is written as null.
func ScriptJSON(v any) template.JS {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf) // escapes <, > and & as < and so on
	if err := enc.Encode(v); err != nil {
		return template.JS("null")
	}
	return template.JS(bytes.TrimSpace(buf.Bytes()))
}
