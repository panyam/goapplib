package hello

import (
	"net/http"

	goal "github.com/panyam/goapplib"
)

// ListPage builds its data from steps: goapplib's mixins read the request (a default body class,
// ?page= and ?q=), and loadItems is the app's own, typed step. LoadAll runs them in order and stops
// at the first that fails or writes the response.
type ListPage struct {
	goal.BasePage
	goal.WithPagination
	goal.WithFiltering
	Header          Header
	NavigationItems []NavItem
	Items           []string
}

func (p *ListPage) Load(r *http.Request, w http.ResponseWriter, app *goal.App[*Site]) (error, bool) {
	return goal.LoadAll(r, w, app, &p.BasePage, &p.WithPagination, &p.WithFiltering,
		goal.LoaderFunc[*Site](p.loadItems))
}

func (p *ListPage) loadItems(r *http.Request, w http.ResponseWriter, app *goal.App[*Site]) (error, bool) {
	p.Title = "Items"
	p.Header.AppName = app.Context.Name
	p.Items = []string{"first item matching " + p.Query}
	return nil, false
}
