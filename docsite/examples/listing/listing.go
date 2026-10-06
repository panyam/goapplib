// Package listing is the app the templates and htmx guides walk through: a page of projects drawn
// with goapplib's EntityListing and Pagination components, whose search answers htmx with only the
// results, a DELETE that updates the count out of band, and an editor page in a BorderLayout.
// listing_test.go drives it over HTTP.
package listing

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	goal "github.com/panyam/goapplib"
)

// Project is one item in the listing. EntityListing reads its Id, Name and Description.
type Project struct {
	Id, Name, Description string
}

// Site is the app context.
type Site struct {
	mu       sync.Mutex
	Projects []Project
}

func (s *Site) search(q string) []Project {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Project
	for _, p := range s.Projects {
		if strings.Contains(strings.ToLower(p.Name), strings.ToLower(q)) {
			out = append(out, p)
		}
	}
	return out
}

// Chrome is what goapplib's layout reads from every page besides BasePage.
type Chrome struct {
	Header struct {
		AppName    string
		IsLoggedIn bool
		Username   string
	}
	NavigationItems []struct {
		Href, Label string
		Active      bool
	}
}

// ProjectsPage is the listing. Its template draws the whole page, and its Results block draws only
// the listing and the pager, which is what an htmx search gets back (WithFragmentTemplate).
type ProjectsPage struct {
	goal.BasePage
	goal.WithPagination
	goal.WithFiltering
	goal.WithHtmx
	Chrome
	Listing *goal.EntityListingData[Project]
}

func (p *ProjectsPage) Load(r *http.Request, w http.ResponseWriter, app *goal.App[*Site]) (error, bool) {
	return goal.LoadAll(r, w, app, &p.BasePage, &p.WithPagination, &p.WithFiltering, &p.WithHtmx,
		goal.LoaderFunc[*Site](p.loadProjects))
}

func (p *ProjectsPage) loadProjects(r *http.Request, w http.ResponseWriter, app *goal.App[*Site]) (error, bool) {
	p.Title = "Projects"
	p.Header.AppName = "Listing"
	all := app.Context.search(p.Query)
	end := min(p.Offset()+p.PageSize, len(all))
	page := all[min(p.Offset(), end):end]
	p.SetTotal(len(all), end < len(all))

	p.Listing = goal.NewEntityListingData[Project]("Projects", "/projects/%s").
		WithDelete("/projects/%s").
		WithHtmx("/projects/")
	p.Listing.Items = page
	p.Listing.EmptyTitle = "No projects match"
	return nil, false
}

// deleteProject removes a project. An htmx request gets an empty 200, an HX-Trigger event, and the
// new count as an out-of-band swap (DeleteResponse in ProjectsPage.html); anyone else is redirected
// back to the list.
func deleteProject(app *goal.App[*Site]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site := app.Context
		site.mu.Lock()
		n := len(site.Projects)
		site.Projects = slices.DeleteFunc(site.Projects, func(p Project) bool { return p.Id == r.PathValue("id") })
		left, deleted := len(site.Projects), len(site.Projects) < n
		site.mu.Unlock()
		if !deleted {
			http.NotFound(w, r)
			return
		}
		if !goal.IsHtmxRequest(r) {
			http.Redirect(w, r, "/projects/", http.StatusSeeOther)
			return
		}
		goal.NewHtmxResponse(w).Trigger("entityUpdated")
		if err := app.RenderTemplate(w, "ProjectsPage", "DeleteResponse", map[string]any{"Count": left}); err != nil {
			http.Error(w, fmt.Sprint(err), http.StatusInternalServerError)
		}
	}
}

// EditorPage is a full-height editor laid out with goapplib's BorderLayout: a toolbar north, a
// status bar south, and the canvas in the center.
type EditorPage struct {
	goal.BasePage
	Chrome
}

func (p *EditorPage) Load(r *http.Request, w http.ResponseWriter, app *goal.App[*Site]) (error, bool) {
	p.Title = "Editor"
	p.CustomHeader = true // the editor's toolbar replaces goapplib's header
	return nil, false
}

// NewHandler registers the pages.
func NewHandler(app *goal.App[*Site]) http.Handler {
	mux := http.NewServeMux()
	goal.Register[*ProjectsPage](app, mux, "GET /projects/{$}", goal.WithFragmentTemplate("ProjectsPage:Results"))
	mux.HandleFunc("DELETE /projects/{id}", deleteProject(app))
	goal.Register[*EditorPage](app, mux, "GET /editor")
	return mux
}
