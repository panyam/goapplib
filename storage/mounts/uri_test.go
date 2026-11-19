package mounts

import (
	"errors"
	"testing"
)

func TestValidName(t *testing.T) {
	for _, name := range []string{"a", "boards", "jetson-2", "x.y_z", "0lib", "a234567890123456789012345678901234567890123456789012345678901234"} {
		if err := ValidName(name); err != nil {
			t.Errorf("ValidName(%q) = %v, want nil", name, err)
		}
	}
	for _, name := range []string{"", "Boards", "-x", ".x", "a/b", `a\b`, "a b", "..", "a2345678901234567890123456789012345678901234567890123456789012345"} {
		if err := ValidName(name); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("ValidName(%q) = %v, want ErrInvalidArgument", name, err)
		}
	}
}

func TestParseURI(t *testing.T) {
	ok := map[string]URI{
		"mount://boards":               {Mount: "boards"},
		"mount://boards/":              {Mount: "boards"},
		"mount://boards/a/b.kicad_sch": {Mount: "boards", Path: "a/b.kicad_sch"},
		"mount://boards/a/./b/../c/":   {Mount: "boards", Path: "a/c"},
		`mount://boards/a\b`:           {Mount: "boards", Path: "a/b"},
		"mount://boards/a/..":          {Mount: "boards"},
	}
	for s, want := range ok {
		got, err := ParseURI(s)
		if err != nil || got != want {
			t.Errorf("ParseURI(%q) = %+v, %v; want %+v", s, got, err, want)
		}
	}
	bad := map[string]error{
		"file:///etc/passwd":       ErrInvalidPath,
		"mount://boards//etc":      ErrInvalidPath,
		"mount://boards/..":        ErrInvalidPath,
		"mount://boards/../x":      ErrInvalidPath,
		"mount://boards/a/../../x": ErrInvalidPath,
		`mount://boards/..\x`:      ErrInvalidPath,
		"mount://Boards/x":         ErrInvalidArgument,
		"mount:///x":               ErrInvalidArgument,
	}
	for s, want := range bad {
		if _, err := ParseURI(s); !errors.Is(err, want) {
			t.Errorf("ParseURI(%q) = %v, want %v", s, err, want)
		}
	}
}

func TestURIRoundTripAndJoin(t *testing.T) {
	u, err := ParseURI("mount://boards/a/b")
	if err != nil {
		t.Fatal(err)
	}
	if back, err := ParseURI(u.String()); err != nil || back != u {
		t.Errorf("ParseURI(%q) = %+v, %v; want %+v", u.String(), back, err, u)
	}
	if j, err := u.Join("c", "d.txt"); err != nil || j.Path != "a/b/c/d.txt" {
		t.Errorf("Join = %+v, %v", j, err)
	}
	if _, err := u.Join("..", "..", ".."); !errors.Is(err, ErrInvalidPath) {
		t.Errorf("Join climbing out = %v, want ErrInvalidPath", err)
	}
}
