package games

import (
	"net/http"

	goal "github.com/panyam/goapplib"
)

// WithGame loads the game named by the route's {id}, for any page about one game. It has
// goapplib's loader shape, so a page chains it with LoadAll: goal.LoadAll(r, w, app, &p.WithGame).
type WithGame struct{ Game Game }

func (m *WithGame) Load(r *http.Request, w http.ResponseWriter, app any) (error, bool) {
	game, ok := app.(*goal.App[*Site]).Context.Store.Get(r.PathValue("id"))
	if !ok {
		http.NotFound(w, r)
		return nil, true
	}
	m.Game = game
	return nil, false
}
