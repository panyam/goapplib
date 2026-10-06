package games

import (
	"net/http"
	"net/url"

	goal "github.com/panyam/goapplib"
)

// Site is the app context every page gets as app.Context.
type Site struct {
	Name  string
	Store *Store
	Auth  goal.AuthProvider
}

// Chrome is what goapplib's layout reads from every page besides BasePage: the header and the
// navigation. Each page embeds it once rather than declaring the fields again.
type Chrome struct {
	Header struct {
		AppName    string
		IsLoggedIn bool
		Username   string
	}
	NavigationItems []NavItem
}

// NavItem is one header link.
type NavItem struct {
	Href, Label string
	Active      bool
}

func (c *Chrome) fill(app *goal.App[*Site], active string) {
	c.Header.AppName = app.Context.Name
	c.NavigationItems = []NavItem{{Href: "/games/", Label: "Games", Active: active == "games"}}
}

// GamesListPage lists games, a page at a time, filtered by ?q=. Its steps run in order through
// LoadAll: goapplib's mixins read the request, then loadGames fetches this page of games and tells
// WithPagination how many there are.
type GamesListPage struct {
	goal.BasePage
	goal.WithPagination
	goal.WithFiltering
	Chrome
	Games []Game
}

func (p *GamesListPage) Load(r *http.Request, w http.ResponseWriter, app *goal.App[*Site]) (error, bool) {
	return goal.LoadAll(r, w, app, &p.BasePage, &p.WithPagination, &p.WithFiltering,
		goal.LoaderFunc[*Site](p.loadGames))
}

func (p *GamesListPage) loadGames(r *http.Request, w http.ResponseWriter, app *goal.App[*Site]) (error, bool) {
	p.Title = "Games"
	p.fill(app, "games")
	games, total := app.Context.Store.List(p.Query, p.Offset(), p.PageSize)
	p.Games = games
	p.SetTotal(total, p.Offset()+len(games) < total)
	return nil, false
}

// GamePage shows one game, by the {id} in its route. An unknown id answers 404 itself and returns
// finished = true, so nothing renders.
type GamePage struct {
	goal.BasePage
	Chrome
	Game Game
}

func (p *GamePage) Load(r *http.Request, w http.ResponseWriter, app *goal.App[*Site]) (error, bool) {
	game, ok := app.Context.Store.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return nil, true
	}
	p.Game = game
	p.Title = game.Name
	p.fill(app, "games")
	return nil, false
}

// NewGamePage needs a login. WithAuth, loaded through AuthLoader, says who's asking, and someone
// who isn't logged in is sent to the login page instead.
type NewGamePage struct {
	goal.BasePage
	goal.WithAuth
	Chrome
}

func (p *NewGamePage) Load(r *http.Request, w http.ResponseWriter, app *goal.App[*Site]) (error, bool) {
	if err, done := goal.LoadAll(r, w, app, &p.BasePage,
		goal.AuthLoader[*Site](&p.WithAuth, app.Context.Auth)); err != nil || done {
		return err, done
	}
	if !p.IsLoggedIn {
		// r.URL.Path is "/new" here, since the group stripped "/games". RequestURI is the path
		// the browser asked for.
		http.Redirect(w, r, "/login?next="+url.QueryEscape(r.RequestURI), http.StatusFound)
		return nil, true
	}
	p.Title = "New game"
	p.fill(app, "games")
	p.Header.IsLoggedIn, p.Header.Username = true, p.Username
	return nil, false
}

// deleteGame is a plain handler beside the pages: no view and no template.
func deleteGame(app *goal.App[*Site]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !app.Context.Store.Delete(r.PathValue("id")) {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/games/", http.StatusSeeOther)
	}
}

// CookieAuth is a stand-in goapplib.AuthProvider: the "user" cookie names who's logged in. A real
// app's provider asks its session store.
type CookieAuth struct{}

func (CookieAuth) GetLoggedInUserId(r *http.Request) string {
	if c, err := r.Cookie("user"); err == nil {
		return c.Value
	}
	return ""
}

func (CookieAuth) GetUserById(id string) (goal.AuthUser, error) { return profile{"username": id}, nil }

type profile map[string]any

func (p profile) Profile() map[string]any { return p }
