package games

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	goal "github.com/panyam/goapplib"
)

func newApp() *goal.App[*Site] {
	store := NewStore(Game{"1", "Chess"}, Game{"2", "Go"}, Game{"3", "Gomoku"}, Game{"4", "Checkers"})
	site := &Site{Name: "Games", Store: store, Auth: CookieAuth{}}
	return goal.NewApp(site, goal.SetupTemplates("templates", "../../../templates"))
}

func do(h http.Handler, method, path, user string) (int, string, string) {
	r := httptest.NewRequest(method, path, nil)
	if user != "" {
		r.AddCookie(&http.Cookie{Name: "user", Value: user})
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	body, _ := io.ReadAll(rec.Body)
	return rec.Code, rec.Header().Get("Location"), string(body)
}

// Both ways of building the routes have to answer the same.
func handlers() map[string]func(*goal.App[*Site]) http.Handler {
	return map[string]func(*goal.App[*Site]) http.Handler{"Register": NewHandler, "MuxBuilder": NewHandlerWithBuilder}
}

func TestTheListPagesAndFilters(t *testing.T) {
	for name, build := range handlers() {
		h := build(newApp())
		code, _, body := do(h, "GET", "/games/?pageSize=2", "")
		if code != 200 || !strings.Contains(body, ">Chess<") || !strings.Contains(body, ">Go<") || strings.Contains(body, ">Gomoku<") ||
			!strings.Contains(body, "4 games.") || !strings.Contains(body, "?page=1") {
			t.Errorf("%s: page 0 of 2 is %d:\n%s", name, code, body)
		}
		if _, _, body := do(h, "GET", "/games/?q=go", ""); !strings.Contains(body, ">Go<") || !strings.Contains(body, ">Gomoku<") || strings.Contains(body, ">Chess<") {
			t.Errorf("%s: ?q=go didn't filter:\n%s", name, body)
		}
	}
}

func TestAGameByItsID(t *testing.T) {
	for name, build := range handlers() {
		h := build(newApp())
		if code, _, body := do(h, "GET", "/games/2", ""); code != 200 || !strings.Contains(body, "<h1>Go</h1>") {
			t.Errorf("%s: /games/2 is %d", name, code)
		}
		if code, _, _ := do(h, "GET", "/games/99", ""); code != 404 {
			t.Errorf("%s: /games/99 is %d, want 404", name, code)
		}
	}
}

func TestTheNewGamePageNeedsALogin(t *testing.T) {
	for name, build := range handlers() {
		h := build(newApp())
		if code, loc, _ := do(h, "GET", "/games/new", ""); code != 302 || loc != "/login?next=%2Fgames%2Fnew" {
			t.Errorf("%s: logged out, /games/new is %d to %q", name, code, loc)
		}
		if code, _, body := do(h, "GET", "/games/new", "ada"); code != 200 || !strings.Contains(body, "Signed in as ada.") {
			t.Errorf("%s: logged in, /games/new is %d", name, code)
		}
	}
}

func TestDeleteRemovesAGame(t *testing.T) {
	for name, build := range handlers() {
		h := build(newApp())
		if code, loc, _ := do(h, "DELETE", "/games/2", ""); code != 303 || loc != "/games/" {
			t.Errorf("%s: DELETE is %d to %q", name, code, loc)
		}
		if code, _, _ := do(h, "GET", "/games/2", ""); code != 404 {
			t.Errorf("%s: after DELETE, /games/2 is %d", name, code)
		}
	}
}
