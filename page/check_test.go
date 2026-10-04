package page

import (
	"reflect"
	"strings"
	"testing"
)

func TestCheckIslandsReportsEachUnknownNameOnceWithItsLayouts(t *testing.T) {
	home := Spec{Layout: "home", Islands: []Island{{Name: "hero", Slot: "top"}, {Name: "chat", Slot: "side"}, {Name: "ghost", Slot: "foot"}}}
	game := Spec{Layout: "game", Islands: []Island{{Name: "board", Slot: "main"}, {Name: "chat", Slot: "side"}}}
	err := CheckIslands([]string{"hero", "board"}, home, game)
	if err == nil {
		t.Fatal("no error for chat and ghost")
	}
	want := `page specs name islands the registry doesn't have: "chat" (in home, game); "ghost" (in home)`
	if err.Error() != want {
		t.Fatalf("got  %s\nwant %s", err, want)
	}
}

func TestCheckIslandsWithEverythingKnown(t *testing.T) {
	s := Spec{Layout: "home", Islands: []Island{{Name: "hero", Slot: "top"}}}
	if err := CheckIslands([]string{"hero", "unused"}, s, s); err != nil {
		t.Fatal(err)
	}
	if err := CheckIslands(nil); err != nil {
		t.Fatalf("no specs: %v", err)
	}
	if err := CheckIslands(nil, Spec{Layout: "empty"}); err != nil {
		t.Fatalf("spec without islands: %v", err)
	}
}

func TestAssetsNames(t *testing.T) {
	a := loadFixture(t, EsbuildOptions{OutDir: "dist", URLPrefix: "/static/"})
	if got, want := a.Names(), []string{"below", "hero", "narrow"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Names: got %v, want %v", got, want)
	}
	var none *Assets
	if got := none.Names(); len(got) != 0 {
		t.Fatalf("nil Assets: %v", got)
	}
	if err := CheckIslands(a.Names(), Spec{Layout: "x", Islands: []Island{{Name: "toolbar", Slot: "top"}}}); err == nil || !strings.Contains(err.Error(), `"toolbar"`) {
		t.Fatalf("a bundled island missing from the known list should be reported: %v", err)
	}
}
