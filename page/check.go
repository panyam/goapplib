package page

import (
	"fmt"
	"slices"
	"strings"
)

// CheckIslands reports every island the specs name that isn't in known, the
// names the browser's registry can mount. The page itself only finds out at
// runtime, with a console warning on whichever page names the missing island,
// so this is for a test that walks an app's specs:
//
//	known := append(assets.Names(), "toolbar") // lazy entries, plus those bundled with the entry
//	if err := page.CheckIslands(known, homeSpec, gameSpec); err != nil {
//		t.Fatal(err)
//	}
//
// It returns nil when every name is known, and otherwise one error listing
// each unknown name once, sorted, with the layouts that use it. It returns an
// error rather than taking a *testing.T so the page package doesn't import
// testing, which would add the test flags to every binary that imports page.
func CheckIslands(known []string, specs ...Spec) error {
	have := map[string]bool{}
	for _, n := range known {
		have[n] = true
	}
	missing := map[string][]string{}
	for _, s := range specs {
		for _, is := range s.Islands {
			if !have[is.Name] && !slices.Contains(missing[is.Name], s.Layout) {
				missing[is.Name] = append(missing[is.Name], s.Layout)
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	names := make([]string, 0, len(missing))
	for n := range missing {
		names = append(names, n)
	}
	slices.Sort(names)
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = fmt.Sprintf("%q (in %s)", n, strings.Join(missing[n], ", "))
	}
	return fmt.Errorf("page specs name islands the registry doesn't have: %s", strings.Join(parts, "; "))
}
