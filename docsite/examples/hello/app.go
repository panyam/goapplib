// Package hello is the app the getting-started guide walks through. The guide includes these
// files as they are, and hello_test.go renders the page, so what a reader copies is what CI built.
package hello

import (
	"net/http"

	goal "github.com/panyam/goapplib"
)

// Site is the app context: what every page can reach, built once at startup and handed to each
// page's Load as app.Context.
type Site struct {
	Name    string
	Tagline string
}

// Header is the data goapplib's Header template reads, as the page's .Header.
type Header struct {
	AppName    string
	IsLoggedIn bool
	Username   string
}

// NavItem is one link in the header's navigation, as goapplib's NavigationTabs template reads it.
type NavItem struct {
	Href   string
	Label  string
	Active bool
}

// HomePage is a page. Embedding goal.BasePage gives it the fields goapplib's BasePage template
// reads (Title, BodyClass, and so on). goapplib's layout also reads .Header and .NavigationItems
// from every page, so a page has those fields too. Load fills them in per request.
type HomePage struct {
	goal.BasePage
	Header          Header
	NavigationItems []NavItem
	Tagline         string
}

// Load runs on every request, before the page renders. Returning true as the second value means
// Load wrote the response itself (a redirect, say), so nothing renders.
func (p *HomePage) Load(r *http.Request, w http.ResponseWriter, app *goal.App[*Site]) (error, bool) {
	p.Title = "Home"
	p.Header.AppName = app.Context.Name
	p.NavigationItems = []NavItem{{Href: "/", Label: "Home", Active: true}}
	p.Tagline = app.Context.Tagline
	return nil, false
}

// NewHandler builds the app: templates from the app's folder first, then goapplib's, and one page
// at "/". goapplibTemplates is where goapplib's templates/ folder is on disk.
func NewHandler(site *Site, appTemplates, goapplibTemplates string) http.Handler {
	templates := goal.SetupTemplates(appTemplates, goapplibTemplates)
	app := goal.NewApp(site, templates)

	mux := http.NewServeMux()
	goal.Register[*HomePage](app, mux, "/{$}")
	return mux
}
