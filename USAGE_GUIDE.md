# goapplib - Go Web Application Library

A lightweight, stdlib-native Go library for building server-rendered web applications with:
- **Composable mixins** for common page behaviors (pagination, auth, filtering)
- **Template hierarchy** with Templar for inheritance and composition
- **HTMX-ready** components for progressive enhancement
- **Responsive patterns** for mobile/desktop layouts

## Table of Contents

1. [Quick Start](#quick-start)
2. [Core Concepts](#core-concepts)
3. [App and ViewContext](#app-and-viewcontext)
4. [Views and Pages](#views-and-pages)
5. [Mixins](#mixins)
6. [Route Registration](#route-registration)
7. [Page Groups](#page-groups)
8. [Templates](#templates)
9. [HTMX Integration](#htmx-integration)
10. [Responsive Patterns](#responsive-patterns)
11. [Template Installation](#template-installation)

---

## Quick Start

```go
package main

import (
    "net/http"
    "github.com/panyam/goapplib"
)

// 1. Define your ViewContext (app-level shared state)
type MyViewContext struct {
    ClientMgr *services.ClientMgr
    Auth      *AuthService
}

// 2. Define a page
type HomePage struct {
    goapplib.BasePage
    goapplib.WithAuth

    FeaturedItems []*Item
}

func (p *HomePage) Load(r *http.Request, w http.ResponseWriter, vc *MyViewContext) (error, bool) {
    // Load mixins
    if err, done := goapplib.LoadAll(r, w, vc, &p.BasePage, &p.WithAuth); done {
        return err, done
    }

    // Page-specific logic
    p.Title = "Home"
    p.FeaturedItems = vc.ClientMgr.GetFeaturedItems()

    return nil, false
}

func main() {
    // 3. Create ViewContext and App
    vc := &MyViewContext{
        ClientMgr: services.NewClientMgr(),
        Auth:      NewAuthService(),
    }

    templates := goapplib.SetupTemplates("./templates")
    app := goapplib.NewApp(vc, templates)

    // 4. Register routes
    mux := http.NewServeMux()

    goapplib.Register[HomePage](app, mux, "/")
    goapplib.Register[LoginPage](app, mux, "/login")
    goapplib.RegisterGroup[GamesGroup](app, mux, "/games")

    // 5. Serve
    http.ListenAndServe(":8080", mux)
}
```

---

## Core Concepts

### Design Principles

1. **stdlib-native**: Everything uses `*http.ServeMux` and `http.Handler`
2. **Composable mixins**: Embed behaviors, chain loading
3. **Template inheritance**: Templar's include/define/block system
4. **Progressive enhancement**: Works without JS, enhanced with HTMX
5. **No magic**: Explicit registration, clear data flow

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│                      http.ServeMux                       │
├─────────────────────────────────────────────────────────┤
│  /games/ ──► GamesGroup (sub-mux)                       │
│     ├─ / ──────────► GameListingPage                    │
│     ├─ /new ───────► StartGamePage                      │
│     └─ /{id}/view ─► GameViewerPage                     │
├─────────────────────────────────────────────────────────┤
│  Page: GameListingPage                                   │
│  ├─ BasePage (mixin)                                     │
│  ├─ WithPagination (mixin)                               │
│  ├─ WithFiltering (mixin)                                │
│  └─ Load() ──► Template: GameListingPage.html           │
└─────────────────────────────────────────────────────────┘
```

---

## App and ViewContext

### ViewContext

ViewContext holds app-level shared state. Unlike per-request context, it's created once at startup and passed to all handlers.

```go
// Your application defines its own ViewContext
type ViewContext struct {
    // Required services
    ClientMgr *services.ClientMgr  // gRPC clients

    // Authentication
    AuthMiddleware *oneauth.Middleware
    AuthService    oneauth.AuthUserStore

    // Optional: HTMX support
    Htmx *goapplib.HtmxContext

    // App-specific config
    AppName    string
    DebugMode  bool
}
```

### App

App wraps your ViewContext and template system:

```go
type App[AC any] struct {
    Context   *AC
    Templates *tmplr.TemplateGroup
}

// Create an app
vc := &ViewContext{...}
templates := goapplib.SetupTemplates("./templates")
app := goapplib.NewApp(vc, templates)
```

### Template Setup

```go
func SetupTemplates(templatePaths ...string) *tmplr.TemplateGroup {
    templates := tmplr.NewTemplateGroup()

    loader := &tmplr.LoaderList{}
    for _, path := range templatePaths {
        loader.AddLoader(tmplr.NewFileSystemLoader(path))
    }
    templates.Loader = loader

    // Add standard functions
    templates.AddFuncs(goapplib.DefaultFuncMap())

    return templates
}

// Usage with override precedence:
templates := goapplib.SetupTemplates(
    "./templates",              // Your app (highest priority)
    "./templates/theme",        // Theme overrides
    "./vendor/.../goapplib/templates",  // Library defaults
)
```

---

## Views and Pages

### View Interface

Every page implements the View interface:

```go
type View[AC any] interface {
    Load(r *http.Request, w http.ResponseWriter, vc *AC) (err error, finished bool)
}
```

- `err`: Error to display (renders error page if non-nil)
- `finished`: If true, response already written (redirect, error, etc.)

### Basic Page Structure

```go
type GameListingPage struct {
    // Embed mixins
    goapplib.BasePage
    goapplib.WithPagination
    goapplib.WithAuth

    // Page-specific data
    Games []*protos.Game
}

func (p *GameListingPage) Load(r *http.Request, w http.ResponseWriter, vc *ViewContext) (error, bool) {
    // 1. Load mixins in chain
    if err, done := goapplib.LoadAll(r, w, vc,
        &p.BasePage,
        &p.WithPagination,
        &p.WithAuth,
    ); done {
        return err, done
    }

    // 2. Set page metadata
    p.Title = "Games"
    p.ActiveTab = "games"

    // 3. Fetch data
    client := vc.ClientMgr.GetGamesSvcClient()
    resp, err := client.ListGames(context.Background(), &protos.ListGamesRequest{
        Pagination: p.WithPagination.ToProto(),
    })
    if err != nil {
        return err, false
    }

    p.Games = resp.Items
    p.WithPagination.SetFromResponse(resp.Pagination)

    return nil, false
}
```

### Handling Redirects and Early Returns

```go
func (p *ProtectedPage) Load(r *http.Request, w http.ResponseWriter, vc *ViewContext) (error, bool) {
    // Check auth
    userId := vc.AuthMiddleware.GetLoggedInUserId(r)
    if userId == "" {
        http.Redirect(w, r, "/login?next="+r.URL.Path, http.StatusFound)
        return nil, true  // finished = true, skip template
    }

    // Continue...
    return nil, false
}
```

---

## Mixins

Mixins are embeddable structs that provide common functionality.

### Available Mixins

#### BasePage

Common page metadata:

```go
type BasePage struct {
    Title              string  // <title> tag
    BodyClass          string  // Body CSS classes
    ActiveTab          string  // Highlight nav tab
    CustomHeader       bool    // Skip default header
    DisableSplashScreen bool
}

func (p *BasePage) Load(r *http.Request, w http.ResponseWriter, vc any) (error, bool) {
    // Set defaults
    if p.BodyClass == "" {
        p.BodyClass = "h-screen flex flex-col bg-gray-50 dark:bg-gray-900"
    }
    return nil, false
}
```

#### WithPagination

Pagination support:

```go
type WithPagination struct {
    CurrentPage int
    PageSize    int
    TotalCount  int
    HasPrevPage bool
    HasNextPage bool
    Pages       []int  // Page numbers to display
}

func (p *WithPagination) Load(r *http.Request, w http.ResponseWriter, vc any) (error, bool) {
    // Parse query params
    p.CurrentPage = intParam(r, "page", 0)
    p.PageSize = intParam(r, "pageSize", 20)
    return nil, false
}

func (p *WithPagination) ToProto() *protos.Pagination {
    return &protos.Pagination{
        PageOffset: int32(p.CurrentPage * p.PageSize),
        PageSize:   int32(p.PageSize),
    }
}

func (p *WithPagination) SetFromResponse(resp *protos.PaginationResponse) {
    p.TotalCount = int(resp.TotalResults)
    p.HasNextPage = resp.HasMore
    p.HasPrevPage = p.CurrentPage > 0
    p.EvalPages()
}
```

#### WithAuth

Authentication info:

```go
type WithAuth struct {
    LoggedInUserId string
    Username       string
    IsLoggedIn     bool
    IsOwner        bool  // For entity pages
}

// Load requires ViewContext with auth
func (p *WithAuth) LoadWithAuth(r *http.Request, authMw *oneauth.Middleware, authSvc oneauth.AuthUserStore) (error, bool) {
    p.LoggedInUserId = authMw.GetLoggedInUserId(r)
    p.IsLoggedIn = p.LoggedInUserId != ""

    if p.IsLoggedIn {
        user, _ := authSvc.GetUserById(p.LoggedInUserId)
        if user != nil {
            p.Username = user.Profile()["username"].(string)
        }
    }
    return nil, false
}
```

#### WithFiltering

Search and sort:

```go
type WithFiltering struct {
    Query    string
    Sort     string
    ViewMode string  // "grid", "table"
}

func (p *WithFiltering) Load(r *http.Request, w http.ResponseWriter, vc any) (error, bool) {
    q := r.URL.Query()
    p.Query = q.Get("q")
    p.Sort = q.Get("sort")
    p.ViewMode = q.Get("view")

    if p.ViewMode == "" {
        p.ViewMode = "table"
    }
    if p.Sort == "" {
        p.Sort = "modified_desc"
    }
    return nil, false
}
```

#### WithHtmx

HTMX request detection:

```go
type WithHtmx struct {
    IsHtmx      bool
    IsBoosted   bool
    Target      string
    Trigger     string
    CurrentURL  string
}

func (p *WithHtmx) Load(r *http.Request, w http.ResponseWriter, vc any) (error, bool) {
    p.IsHtmx = r.Header.Get("HX-Request") == "true"
    p.IsBoosted = r.Header.Get("HX-Boosted") == "true"
    p.Target = r.Header.Get("HX-Target")
    p.Trigger = r.Header.Get("HX-Trigger")
    p.CurrentURL = r.Header.Get("HX-Current-URL")
    return nil, false
}

// Use in templates: {{ if .WithHtmx.IsHtmx }}...{{ end }}
```

### LoadAll Helper

Chain multiple mixins:

```go
func LoadAll[AC any](r *http.Request, w http.ResponseWriter, vc *AC, loaders ...Loader[AC]) (error, bool) {
    for _, loader := range loaders {
        if err, done := loader.Load(r, w, vc); done || err != nil {
            return err, done
        }
    }
    return nil, false
}

// Loader interface
type Loader[AC any] interface {
    Load(r *http.Request, w http.ResponseWriter, vc *AC) (error, bool)
}
```

### Custom Mixins

Create your own:

```go
type WithGameContext struct {
    GameId    string
    Game      *protos.Game
    GameState *protos.GameState
}

func (p *WithGameContext) Load(r *http.Request, w http.ResponseWriter, vc *ViewContext) (error, bool) {
    p.GameId = r.PathValue("gameId")
    if p.GameId == "" {
        http.Error(w, "Game ID required", http.StatusBadRequest)
        return nil, true
    }

    client := vc.ClientMgr.GetGamesSvcClient()
    resp, err := client.GetGame(context.Background(), &protos.GetGameRequest{Id: p.GameId})
    if err != nil {
        return err, false
    }

    p.Game = resp.Game
    p.GameState = resp.State
    return nil, false
}
```

---

## Route Registration

### Register Function

Register a single page:

```go
func Register[V View[AC], AC any](
    app *App[AC],
    mux *http.ServeMux,
    pattern string,
    opts ...Option,
) *http.ServeMux

// Usage
mux := http.NewServeMux()
goapplib.Register[HomePage](app, mux, "/")
goapplib.Register[GameListingPage](app, mux, "/games/")
goapplib.Register[GameViewerPage](app, mux, "/games/{gameId}/view")
```

### Options

```go
// Override template name
goapplib.Register[GameViewerPage](app, mux, "/games/{gameId}/view",
    goapplib.WithTemplate("GameViewerPageMobile"),
)

// Multiple options
goapplib.Register[GameViewerPage](app, mux, "/games/{gameId}/view",
    goapplib.WithTemplate("CustomTemplate"),
    goapplib.WithMiddleware(authRequired),
)
```

### Custom Handlers

Use stdlib directly for non-View handlers:

```go
// Custom handler function
mux.HandleFunc("DELETE /games/{gameId}", func(w http.ResponseWriter, r *http.Request) {
    gameId := r.PathValue("gameId")
    client := vc.ClientMgr.GetGamesSvcClient()
    _, err := client.DeleteGame(context.Background(), &protos.DeleteGameRequest{Id: gameId})
    if err != nil {
        http.Error(w, err.Error(), 500)
        return
    }
    http.Redirect(w, r, "/games/", http.StatusFound)
})

// Static files
mux.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("./static"))))
```

### RegisterFunc and RegisterHandler

Convenience wrappers:

```go
// For http.HandlerFunc
goapplib.RegisterFunc(mux, "/api/health", func(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("ok"))
})

// For http.Handler
goapplib.RegisterHandler(mux, "/static/",
    http.StripPrefix("/static", http.FileServer(http.Dir("./static"))),
)
```

---

## Page Groups

Groups organize related pages under a common prefix.

### Defining a Group

```go
type GamesGroup struct{}

func (g *GamesGroup) RegisterRoutes(app *goapplib.App[*ViewContext]) *http.ServeMux {
    mux := http.NewServeMux()

    // Pages (patterns are relative - prefix stripped)
    goapplib.Register[GameListingPage](app, mux, "/")
    goapplib.Register[StartGamePage](app, mux, "/new")
    goapplib.Register[GameViewerPage](app, mux, "/{gameId}/view")
    goapplib.Register[GameDetailPage](app, mux, "/{gameId}")

    // Custom handlers
    mux.HandleFunc("/{gameId}/copy", func(w http.ResponseWriter, r *http.Request) {
        gameId := r.PathValue("gameId")
        http.Redirect(w, r, "/games/new?copyFrom="+gameId, http.StatusFound)
    })

    mux.HandleFunc("DELETE /{gameId}", deleteGameHandler)

    return mux
}
```

### Registering a Group

```go
// RegisterGroup mounts the group's mux under a prefix
goapplib.RegisterGroup[GamesGroup](app, rootMux, "/games")

// Results in:
//   /games/           → GameListingPage
//   /games/new        → StartGamePage
//   /games/{id}/view  → GameViewerPage
//   /games/{id}       → GameDetailPage (GET) or deleteHandler (DELETE)
```

### Nested Groups

```go
type AdminGroup struct{}

func (g *AdminGroup) RegisterRoutes(app *goapplib.App[*ViewContext]) *http.ServeMux {
    mux := http.NewServeMux()

    goapplib.Register[AdminDashboard](app, mux, "/")

    // Nested groups
    goapplib.RegisterGroup[AdminUsersGroup](app, mux, "/users")
    goapplib.RegisterGroup[AdminSettingsGroup](app, mux, "/settings")

    return mux
}

// Register at root
goapplib.RegisterGroup[AdminGroup](app, rootMux, "/admin")

// Results in:
//   /admin/
//   /admin/users/
//   /admin/users/{id}
//   /admin/settings/
```

---

## MuxBuilder (Fluent API)

Alternative fluent style for route building:

```go
rootMux := app.NewMux().
    Page("/", func() goapplib.View[*AC] { return &HomePage{} }).
    Page("/login", func() goapplib.View[*AC] { return &LoginPage{} }).

    Group("/games", func(m *goapplib.MuxBuilder[*AC]) {
        m.Page("/", func() goapplib.View[*AC] { return &GameListingPage{} }).
          Page("/new", func() goapplib.View[*AC] { return &StartGamePage{} }).
          Page("/{gameId}/view", func() goapplib.View[*AC] { return &GameViewerPage{} }).
          HandleFunc("DELETE /{gameId}", deleteGameHandler)
    }).

    Group("/worlds", func(m *goapplib.MuxBuilder[*AC]) {
        m.Page("/", func() goapplib.View[*AC] { return &WorldListingPage{} }).
          Page("/{worldId}/view", func() goapplib.View[*AC] { return &WorldViewerPage{} })
    }).

    Handler("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("./static")))).

    Build()

http.ListenAndServe(":8080", rootMux)
```

### MuxBuilder Methods

```go
type MuxBuilder[AC any] struct {...}

// Add a View-based page
func (b *MuxBuilder[AC]) Page(pattern string, maker func() View[AC], opts ...Option) *MuxBuilder[AC]

// Add a nested group
func (b *MuxBuilder[AC]) Group(prefix string, setup func(*MuxBuilder[AC])) *MuxBuilder[AC]

// Add stdlib handler
func (b *MuxBuilder[AC]) Handler(pattern string, h http.Handler) *MuxBuilder[AC]
func (b *MuxBuilder[AC]) HandleFunc(pattern string, h http.HandlerFunc) *MuxBuilder[AC]

// Build the final mux
func (b *MuxBuilder[AC]) Build() *http.ServeMux
```

---

## Templates

### Template Hierarchy

Templates use Templar's include/define/block system:

```
templates/
├── BasePage.html           # Root layout
├── Header.html             # Navigation header
├── components/
│   ├── Pagination.html
│   ├── EntityGrid.html
│   ├── EntityTable.html
│   ├── SearchFilter.html
│   ├── Modal.html
│   ├── Drawer.html
│   └── Toast.html
├── GameListingPage.html    # Extends BasePage
├── GameViewerPage.html
└── ...
```

### BasePage.html

```html
{{# include "Header.html" #}}
{{# include "components/Modal.html" #}}
{{# include "components/Toast.html" #}}

{{ define "BasePage" }}
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{ .Title }}</title>
    <link href="/static/css/tailwind.css" rel="stylesheet">
    <script src="https://unpkg.com/htmx.org@1.9.10" defer></script>
    {{ block "ExtraHeadSection" . }}{{ end }}
</head>
<body class="{{ .BodyClass }}">
    {{ block "HeaderSection" . }}
        {{ if not .CustomHeader }}
        {{ template "Header" .Header }}
        {{ end }}
    {{ end }}

    {{ block "BodySection" . }}{{ end }}

    {{ template "ModalContainer" . }}
    {{ template "ToastContainer" . }}

    {{ block "FooterSection" . }}{{ end }}
    {{ block "ScriptsSection" . }}{{ end }}
</body>
</html>
{{ end }}
```

### Page Template (Extending BasePage)

```html
<!-- GameListingPage.html -->
{{# include "BasePage.html" #}}
{{# include "components/EntityGrid.html" #}}
{{# include "components/Pagination.html" #}}

{{ define "BodySection" }}
<main class="max-w-7xl mx-auto px-4 py-8">
    <div class="mb-8">
        <h1 class="text-3xl font-bold">Games</h1>
        <p class="text-gray-600">Browse and manage your games</p>
    </div>

    {{ template "EntityGrid" (dict "Items" .Games "ItemTemplate" "GameCard") }}
    {{ template "Pagination" .WithPagination }}
</main>
{{ end }}

{{ define "GameListingPage" }}
{{ template "BasePage" . }}
{{ end }}
```

### Reusable Components

```html
<!-- components/EntityGrid.html -->
{{ define "EntityGrid" }}
<div id="{{ .ContainerId | default "entity-grid" }}"
     class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
    {{ range .Items }}
    {{ block "GridItem" . }}
        <div class="entity-card bg-white dark:bg-gray-800 rounded-lg shadow">
            {{ block "GridItemContent" . }}{{ end }}
        </div>
    {{ end }}
    {{ end }}
</div>

{{ if not .Items }}
{{ template "EmptyState" . }}
{{ end }}
{{ end }}
```

### Overriding Blocks

```html
<!-- Your app's GameListingPage.html -->
{{# include "goapplib/EntityListingPage.html" #}}

{{ define "GridItem" }}
<div class="game-card">
    <img src="{{ .PreviewUrl }}" alt="{{ .Name }}">
    <h3>{{ .Name }}</h3>
    <p>{{ .Description }}</p>
    <a href="/games/{{ .Id }}/view" class="btn-primary">Play</a>
</div>
{{ end }}

{{ define "ListingTitle" }}My Games{{ end }}

{{ define "GameListingPage" }}
{{ template "EntityListingPage" . }}
{{ end }}
```

---

## HTMX Integration

### HTMX-Aware Templates

Components can adapt based on HTMX context:

```html
<!-- SearchFilter with HTMX -->
{{ define "SearchFilter" }}
<input type="search"
       name="q"
       value="{{ .Query }}"
       placeholder="Search..."
       class="input-search"
       {{ if .WithHtmx }}
       hx-get="{{ .SearchUrl }}"
       hx-trigger="keyup changed delay:300ms"
       hx-target="{{ .TargetSelector }}"
       hx-swap="innerHTML"
       hx-push-url="true"
       {{ else }}
       onchange="this.form.submit()"
       {{ end }}>
{{ end }}
```

### Fragment Rendering

Same endpoint can return full page or fragment:

```go
func (p *GameListingPage) Load(r *http.Request, w http.ResponseWriter, vc *ViewContext) (error, bool) {
    // Load including HTMX mixin
    goapplib.LoadAll(r, w, vc, &p.BasePage, &p.WithHtmx, &p.WithPagination)

    // Fetch data...

    return nil, false
}

// In handler, choose template based on HTMX
func smartHandler(app *App, fullTemplate, fragmentTemplate string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        view := &GameListingPage{}
        view.Load(r, w, app.Context)

        template := fullTemplate
        if view.WithHtmx.IsHtmx && !view.WithHtmx.IsBoosted {
            template = fragmentTemplate
        }

        app.RenderTemplate(w, template, view)
    }
}
```

### HTMX Response Helpers

```go
type HtmxResponse struct {
    w http.ResponseWriter
}

func NewHtmxResponse(w http.ResponseWriter) *HtmxResponse {
    return &HtmxResponse{w: w}
}

func (h *HtmxResponse) Trigger(event string)         { h.w.Header().Set("HX-Trigger", event) }
func (h *HtmxResponse) Redirect(url string)          { h.w.Header().Set("HX-Redirect", url) }
func (h *HtmxResponse) Refresh()                     { h.w.Header().Set("HX-Refresh", "true") }
func (h *HtmxResponse) PushURL(url string)           { h.w.Header().Set("HX-Push-Url", url) }
func (h *HtmxResponse) ReplaceURL(url string)        { h.w.Header().Set("HX-Replace-Url", url) }
func (h *HtmxResponse) Retarget(selector string)     { h.w.Header().Set("HX-Retarget", selector) }
func (h *HtmxResponse) Reswap(style string)          { h.w.Header().Set("HX-Reswap", style) }

// Usage
func deleteHandler(w http.ResponseWriter, r *http.Request) {
    // ... delete logic ...

    if r.Header.Get("HX-Request") == "true" {
        hx := goapplib.NewHtmxResponse(w)
        hx.Trigger("entityDeleted")
        w.WriteHeader(200)
        return
    }

    http.Redirect(w, r, "/games/", http.StatusFound)
}
```

### OOB (Out-of-Band) Updates

```html
<!-- DeleteResponse.html - updates multiple elements -->
{{ define "DeleteResponse" }}
{{/* Primary: remove deleted item */}}
<div id="item-{{ .DeletedId }}"></div>

{{/* OOB: update count */}}
<span id="item-count" hx-swap-oob="true">
    {{ .RemainingCount }} items
</span>

{{/* OOB: show toast */}}
<div id="toast-container" hx-swap-oob="beforeend">
    {{ template "Toast" (dict "Type" "success" "Message" "Deleted successfully") }}
</div>
{{ end }}
```

---

## Responsive Patterns

### CSS-Based (Recommended for most cases)

Use Tailwind breakpoints:

```html
<div class="
    grid grid-cols-1      {{/* Mobile: 1 column */}}
    sm:grid-cols-2        {{/* Tablet: 2 columns */}}
    lg:grid-cols-3        {{/* Desktop: 3 columns */}}
    xl:grid-cols-4        {{/* Large: 4 columns */}}
    gap-4
">
```

### Mobile Bottom Bar

```html
{{ define "MobileBottomBar" }}
<nav class="fixed bottom-0 inset-x-0 h-16 bg-white border-t
            flex items-center justify-around
            md:hidden {{/* Hide on desktop */}}">
    {{ range .BottomBarItems }}
    <button class="flex flex-col items-center p-2" data-action="{{ .Action }}">
        {{ .Icon | safeHTML }}
        <span class="text-xs">{{ .Label }}</span>
    </button>
    {{ end }}
</nav>
{{ end }}
```

### Mobile Drawer

```html
{{ define "Drawer" }}
<div id="drawer-{{ .Id }}" class="drawer-overlay fixed inset-0 z-40 hidden">
    <div class="drawer-backdrop absolute inset-0 bg-black/50" onclick="closeDrawer('{{ .Id }}')"></div>
    <div class="drawer-panel absolute bottom-0 inset-x-0 h-[70vh]
                bg-white rounded-t-xl shadow-2xl
                transform translate-y-full transition-transform">
        <div class="p-4">
            {{ block "DrawerContent" . }}{{ end }}
        </div>
    </div>
</div>
{{ end }}
```

### Server-Side Layout Detection (Optional)

For complex layouts that differ significantly:

```go
func gameViewerHandler(app *App, w http.ResponseWriter, r *http.Request) {
    layout := detectLayout(r)  // "mobile", "tablet", "desktop"

    templates := map[string]string{
        "mobile":  "GameViewerPageMobile",
        "tablet":  "GameViewerPageGrid",
        "desktop": "GameViewerPageDockView",
    }

    view := &GameViewerPage{}
    view.Load(r, w, app.Context)
    app.RenderTemplate(w, templates[layout], view)
}

func detectLayout(r *http.Request) string {
    // 1. Query param: ?layout=mobile
    if layout := r.URL.Query().Get("layout"); layout != "" {
        return layout
    }
    // 2. Cookie preference
    if cookie, err := r.Cookie("layout"); err == nil {
        return cookie.Value
    }
    // 3. User-Agent detection (optional)
    // 4. Default
    return "desktop"
}
```

---

## Template Installation

Since Go modules can't directly serve static files to dependents, use one of these approaches:

### Option 1: Vendor Path

```bash
# Your project structure
myapp/
├── go.mod
├── vendor/
│   └── github.com/panyam/goapplib/
│       └── templates/
└── templates/           # Your overrides
```

```go
templates := goapplib.SetupTemplates(
    "./templates",                                    // Your overrides
    "./vendor/github.com/panyam/goapplib/templates",   // Library defaults
)
```

### Option 2: Symlink

```makefile
# Makefile
WEBLIB_PATH := $(shell go list -m -f '{{.Dir}}' github.com/panyam/goapplib)

setup:
    ln -sf $(WEBLIB_PATH)/templates ./templates/goapplib
```

### Option 3: Copy/Eject

```bash
# Copy templates to your project (for customization)
go run github.com/panyam/goapplib/cmd/eject-templates ./templates/lib
```

### Option 4: Embedded (if needed)

```go
// In goapplib, if embedding is acceptable:
//go:embed templates/*
var EmbeddedTemplates embed.FS

// Usage
templates.Loader = (&tmplr.LoaderList{}).
    AddLoader(tmplr.NewFileSystemLoader("./templates")).
    AddLoader(tmplr.NewEmbedLoader(goapplib.EmbeddedTemplates, "templates"))
```

---

## Complete Example

```go
package main

import (
    "context"
    "net/http"

    "github.com/panyam/goapplib"
    "myapp/services"
    protos "myapp/gen/go/myapp/v1"
)

// ViewContext - app-level shared state
type ViewContext struct {
    ClientMgr      *services.ClientMgr
    AuthMiddleware *oneauth.Middleware
    AuthService    oneauth.AuthUserStore
}

// GameListingPage
type GameListingPage struct {
    goapplib.BasePage
    goapplib.WithPagination
    goapplib.WithFiltering
    goapplib.WithAuth
    goapplib.WithHtmx

    Games []*protos.Game
}

func (p *GameListingPage) Load(r *http.Request, w http.ResponseWriter, vc *ViewContext) (error, bool) {
    // Load mixins
    if err, done := goapplib.LoadAll(r, w, vc,
        &p.BasePage,
        &p.WithPagination,
        &p.WithFiltering,
        goapplib.AuthLoader(&p.WithAuth, vc.AuthMiddleware, vc.AuthService),
        &p.WithHtmx,
    ); done {
        return err, done
    }

    p.Title = "Games"
    p.ActiveTab = "games"

    // Fetch games
    client := vc.ClientMgr.GetGamesSvcClient()
    resp, err := client.ListGames(context.Background(), &protos.ListGamesRequest{
        Pagination: p.WithPagination.ToProto(),
        Query:      p.WithFiltering.Query,
        Sort:       p.WithFiltering.Sort,
    })
    if err != nil {
        return err, false
    }

    p.Games = resp.Items
    p.WithPagination.SetFromResponse(resp.Pagination)

    return nil, false
}

// GamesGroup
type GamesGroup struct{}

func (g *GamesGroup) RegisterRoutes(app *goapplib.App[*ViewContext]) *http.ServeMux {
    mux := http.NewServeMux()

    goapplib.Register[GameListingPage](app, mux, "/")
    goapplib.Register[StartGamePage](app, mux, "/new")
    goapplib.Register[GameViewerPage](app, mux, "/{gameId}/view")

    mux.HandleFunc("DELETE /{gameId}", func(w http.ResponseWriter, r *http.Request) {
        gameId := r.PathValue("gameId")
        client := app.Context.ClientMgr.GetGamesSvcClient()
        client.DeleteGame(context.Background(), &protos.DeleteGameRequest{Id: gameId})

        if r.Header.Get("HX-Request") == "true" {
            goapplib.NewHtmxResponse(w).Trigger("gameDeleted")
            return
        }
        http.Redirect(w, r, "/games/", http.StatusFound)
    })

    return mux
}

func main() {
    // Setup
    vc := &ViewContext{
        ClientMgr:      services.NewClientMgr(),
        AuthMiddleware: setupAuth(),
        AuthService:    setupAuthService(),
    }

    templates := goapplib.SetupTemplates("./templates", "./vendor/.../goapplib/templates")
    app := goapplib.NewApp(vc, templates)

    // Routes
    mux := http.NewServeMux()

    goapplib.Register[HomePage](app, mux, "/")
    goapplib.Register[LoginPage](app, mux, "/login")
    goapplib.RegisterGroup[GamesGroup](app, mux, "/games")
    goapplib.RegisterGroup[WorldsGroup](app, mux, "/worlds")

    mux.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("./static"))))

    // Serve
    http.ListenAndServe(":8080", mux)
}
```

---

## API Reference

### Core Types

```go
type App[AC any] struct { ... }
type View[AC any] interface { Load(...) (error, bool) }
type Loader[AC any] interface { Load(...) (error, bool) }
type PageGroup[AC any] interface { RegisterRoutes(*App[AC]) *http.ServeMux }
type Option func(*options)
```

### Functions

```go
func NewApp[AC any](vc *AC, templates *tmplr.TemplateGroup) *App[AC]
func SetupTemplates(paths ...string) *tmplr.TemplateGroup
func Register[V View[AC], AC any](app *App[AC], mux *http.ServeMux, pattern string, opts ...Option) *http.ServeMux
func RegisterGroup[G PageGroup[AC], AC any](app *App[AC], mux *http.ServeMux, prefix string, opts ...Option) *http.ServeMux
func RegisterFunc(mux *http.ServeMux, pattern string, handler http.HandlerFunc) *http.ServeMux
func RegisterHandler(mux *http.ServeMux, pattern string, handler http.Handler) *http.ServeMux
func LoadAll[AC any](r *http.Request, w http.ResponseWriter, vc *AC, loaders ...Loader[AC]) (error, bool)
```

### Mixins

```go
type BasePage struct { ... }
type WithPagination struct { ... }
type WithFiltering struct { ... }
type WithAuth struct { ... }
type WithHtmx struct { ... }
```

### HTMX Helpers

```go
type HtmxResponse struct { ... }
func NewHtmxResponse(w http.ResponseWriter) *HtmxResponse
func (h *HtmxResponse) Trigger(event string)
func (h *HtmxResponse) Redirect(url string)
func (h *HtmxResponse) Refresh()
func (h *HtmxResponse) PushURL(url string)
func (h *HtmxResponse) ReplaceURL(url string)
func (h *HtmxResponse) Retarget(selector string)
func (h *HtmxResponse) Reswap(style string)
```
