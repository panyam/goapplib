package goapplib

import (
	"net/http"
	"strconv"
)

// BasePage provides common page metadata.
// Embed this in your page structs.
type BasePage struct {
	Title               string // Page title for <title> tag
	BodyClass           string // CSS classes for <body>
	ActiveTab           string // Highlight active navigation tab
	CustomHeader        bool   // If true, skip default header rendering
	DisableSplashScreen bool   // If true, hide loading splash
	SplashTitle         string // Custom splash title
	SplashMessage       string // Custom splash message
	BodyDataAttributes  string // HTML data attributes for body
}

// Load implements Loader for BasePage.
// Note: This is a generic implementation that works with any ViewContext.
func (p *BasePage) Load(r *http.Request, w http.ResponseWriter, vc any) (error, bool) {
	if p.BodyClass == "" {
		p.BodyClass = "h-screen flex flex-col bg-gray-50 dark:bg-gray-900 text-gray-900 dark:text-gray-100"
	}
	return nil, false
}

// WithPagination provides pagination support.
// Embed this in page structs that display paginated lists.
type WithPagination struct {
	CurrentPage int   // 0-indexed current page
	PageSize    int   // Items per page
	TotalCount  int   // Total number of items
	HasPrevPage bool  // True if there's a previous page
	HasNextPage bool  // True if there's a next page
	Pages       []int // Page numbers to display in pagination UI
}

// Load implements Loader for WithPagination.
func (p *WithPagination) Load(r *http.Request, w http.ResponseWriter, vc any) (error, bool) {
	p.CurrentPage = intQueryParam(r, "page", 0)
	p.PageSize = intQueryParam(r, "pageSize", 20)

	// Ensure valid values
	if p.CurrentPage < 0 {
		p.CurrentPage = 0
	}
	if p.PageSize < 1 {
		p.PageSize = 20
	}
	if p.PageSize > 100 {
		p.PageSize = 100
	}

	return nil, false
}

// SetTotal updates pagination state based on total count.
func (p *WithPagination) SetTotal(total int, hasMore bool) {
	p.TotalCount = total
	p.HasNextPage = hasMore
	p.HasPrevPage = p.CurrentPage > 0
	p.EvalPages()
}

// EvalPages calculates page numbers to display.
func (p *WithPagination) EvalPages() {
	p.Pages = nil
	if p.TotalCount <= 0 || p.PageSize <= 0 {
		return
	}

	totalPages := (p.TotalCount + p.PageSize - 1) / p.PageSize
	if totalPages <= 1 {
		return
	}

	// Show up to 5 pages centered on current
	start := p.CurrentPage - 2
	if start < 0 {
		start = 0
	}
	end := start + 5
	if end > totalPages {
		end = totalPages
		start = end - 5
		if start < 0 {
			start = 0
		}
	}

	for i := start; i < end; i++ {
		p.Pages = append(p.Pages, i)
	}
}

// Offset returns the offset for database queries.
func (p *WithPagination) Offset() int {
	return p.CurrentPage * p.PageSize
}

// PrevPage returns the previous page number.
func (p *WithPagination) PrevPage() int {
	if p.CurrentPage > 0 {
		return p.CurrentPage - 1
	}
	return 0
}

// NextPage returns the next page number.
func (p *WithPagination) NextPage() int {
	return p.CurrentPage + 1
}

// Paginator returns self for template access via .Paginator
// This allows templates to use {{ .Paginator.HasPrevPage }} etc.
// when the page struct embeds WithPagination.
func (p *WithPagination) Paginator() *WithPagination {
	return p
}

// WithFiltering provides search and sort support.
// Embed this in page structs that support filtering.
type WithFiltering struct {
	Query    string // Search query
	Sort     string // Sort field/direction
	ViewMode string // Display mode: "grid", "table", etc.
}

// Load implements Loader for WithFiltering.
func (p *WithFiltering) Load(r *http.Request, w http.ResponseWriter, vc any) (error, bool) {
	q := r.URL.Query()
	p.Query = q.Get("q")
	p.Sort = q.Get("sort")
	p.ViewMode = q.Get("view")

	// Defaults
	if p.ViewMode == "" {
		p.ViewMode = "table"
	}
	if p.Sort == "" {
		p.Sort = "modified_desc"
	}

	return nil, false
}

// WithAuth provides authentication info.
// Embed this in page structs that need user info.
type WithAuth struct {
	LoggedInUserId string // Current user's ID
	Username       string // Current user's display name
	IsLoggedIn     bool   // True if user is authenticated
	IsOwner        bool   // True if user owns the current entity
}

// Load is a no-op for WithAuth.
// Use LoadWithAuth to load auth info with your auth services.
func (p *WithAuth) Load(r *http.Request, w http.ResponseWriter, vc any) (error, bool) {
	// This is a no-op. Use AuthLoader or LoadWithAuth instead.
	return nil, false
}

// AuthProvider is the interface for auth services.
type AuthProvider interface {
	GetLoggedInUserId(r *http.Request) string
	GetUserById(id string) (AuthUser, error)
}

// AuthUser is a minimal user interface.
type AuthUser interface {
	Profile() map[string]any
}

// LoadWithAuth loads auth info using the provided auth services.
func (p *WithAuth) LoadWithAuth(r *http.Request, provider AuthProvider) (error, bool) {
	p.LoggedInUserId = provider.GetLoggedInUserId(r)
	p.IsLoggedIn = p.LoggedInUserId != ""

	if p.IsLoggedIn {
		user, err := provider.GetUserById(p.LoggedInUserId)
		if err == nil && user != nil {
			profile := user.Profile()
			if username, ok := profile["username"].(string); ok {
				p.Username = username
			}
		}
	}

	return nil, false
}

// AuthLoader returns a LoaderFunc that loads auth info.
// Use this with LoadAll when you have an AuthProvider.
func AuthLoader[AC any](auth *WithAuth, provider AuthProvider) LoaderFunc[AC] {
	return func(r *http.Request, w http.ResponseWriter, app *App[AC]) (error, bool) {
		return auth.LoadWithAuth(r, provider)
	}
}

// WithHtmx provides HTMX request detection.
// Embed this in page structs that need HTMX-aware rendering.
type WithHtmx struct {
	IsHtmx      bool   // True if this is an HTMX request
	IsBoosted   bool   // True if this is a boosted link
	Target      string // HX-Target header value
	Trigger     string // HX-Trigger header value
	TriggerName string // HX-Trigger-Name header value
	CurrentURL  string // HX-Current-URL header value
	Prompt      string // HX-Prompt header value
}

// Load implements Loader for WithHtmx.
func (p *WithHtmx) Load(r *http.Request, w http.ResponseWriter, vc any) (error, bool) {
	p.IsHtmx = r.Header.Get("HX-Request") == "true"
	p.IsBoosted = r.Header.Get("HX-Boosted") == "true"
	p.Target = r.Header.Get("HX-Target")
	p.Trigger = r.Header.Get("HX-Trigger")
	p.TriggerName = r.Header.Get("HX-Trigger-Name")
	p.CurrentURL = r.Header.Get("HX-Current-URL")
	p.Prompt = r.Header.Get("HX-Prompt")
	return nil, false
}

// ShouldRenderFragment returns true if only a fragment should be rendered.
func (p *WithHtmx) ShouldRenderFragment() bool {
	return p.IsHtmx && !p.IsBoosted
}

// Helper functions

func intQueryParam(r *http.Request, name string, defaultVal int) int {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return i
}

func stringQueryParam(r *http.Request, name string, defaultVal string) string {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultVal
	}
	return val
}
