package dsmigrate

import (
	"testing"

	"cloud.google.com/go/datastore"
)

// Datastore doesn't promise an order for a nested entity's properties, and two reads of the
// same unchanged entity can return them differently. WriteBack's check must treat those as
// equal, or every entity with a nested entity looks changed and never gets written back.
func TestSameProperties_NestedEntityOrder(t *testing.T) {
	nested := func(names ...string) *datastore.Entity {
		e := &datastore.Entity{}
		for _, n := range names {
			e.Properties = append(e.Properties, datastore.Property{Name: n, Value: "v-" + n})
		}
		return e
	}
	a := datastore.PropertyList{
		{Name: "isActive", Value: true},
		{Name: "profile", Value: nested("name", "email", "picture")},
		{Name: "tags", Value: []any{nested("x", "y"), "plain"}},
	}
	b := datastore.PropertyList{
		{Name: "tags", Value: []any{nested("y", "x"), "plain"}},
		{Name: "profile", Value: nested("picture", "name", "email")},
		{Name: "isActive", Value: true},
	}
	if !sameProperties(a, b) {
		t.Error("the same entity with nested properties in another order compared different")
	}

	changed := datastore.PropertyList{
		{Name: "isActive", Value: true},
		{Name: "profile", Value: &datastore.Entity{Properties: []datastore.Property{{Name: "name", Value: "someone else"}, {Name: "email", Value: "v-email"}, {Name: "picture", Value: "v-picture"}}}},
		{Name: "tags", Value: []any{nested("x", "y"), "plain"}},
	}
	if sameProperties(a, changed) {
		t.Error("a changed nested value compared equal")
	}
	reordered := datastore.PropertyList{
		{Name: "isActive", Value: true},
		{Name: "profile", Value: nested("name", "email", "picture")},
		{Name: "tags", Value: []any{"plain", nested("x", "y")}},
	}
	if sameProperties(a, reordered) {
		t.Error("reordering a multi-valued property's values compared equal; their order is meaningful")
	}
}
