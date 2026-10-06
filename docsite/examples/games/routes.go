package games

import (
	"net/http"

	goal "github.com/panyam/goapplib"
)

// GamesGroup is every route under /games. RegisterGroup strips the prefix before these match, so
// "/{id}" answers /games/7.
type GamesGroup struct{}

func (GamesGroup) RegisterRoutes(app *goal.App[*Site]) *http.ServeMux {
	mux := http.NewServeMux()
	goal.Register[*GamesListPage](app, mux, "GET /{$}")
	goal.Register[*NewGamePage](app, mux, "GET /new")
	goal.Register[*GamePage](app, mux, "GET /{id}")
	mux.HandleFunc("DELETE /{id}", deleteGame(app))
	return mux
}

// NewHandler builds the app with Register and RegisterGroup.
func NewHandler(app *goal.App[*Site]) http.Handler {
	mux := http.NewServeMux()
	goal.RegisterGroup[*GamesGroup](app, mux, "/games")
	return mux
}

// NewHandlerWithBuilder builds the same routes with MuxBuilder.
func NewHandlerWithBuilder(app *goal.App[*Site]) http.Handler {
	return app.NewMux().
		Group("/games", func(m *goal.MuxBuilder[*Site]) {
			m.Page("GET /{$}", func() goal.View[*Site] { return &GamesListPage{} }).
				Page("GET /new", func() goal.View[*Site] { return &NewGamePage{} }).
				Page("GET /{id}", func() goal.View[*Site] { return &GamePage{} }).
				HandleFunc("DELETE /{id}", deleteGame(app))
		}).
		Build()
}
