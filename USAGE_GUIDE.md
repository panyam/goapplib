# goapplib - Go Web Application Library

A lightweight, stdlib-native Go library for building server-rendered web applications with:
- **Composable mixins** for common page behaviors (pagination, auth, filtering)
- **Template hierarchy** with Templar for inheritance and composition
- **HTMX-ready** components for progressive enhancement
- **Responsive patterns** for mobile/desktop layouts

## Table of Contents

1. [Quick Start, Core Concepts, App and ViewContext](#quick-start-core-concepts-app-and-viewcontext) (moved to the docs site)
4. [Views and Pages](#views-and-pages)
5. [Mixins](#mixins)
6. [Route Registration](#route-registration)
7. [Page Groups](#page-groups)
8. [Templates](#templates)
9. [BorderLayout](#borderlayout)
10. [Island Pages](#island-pages)
11. [HTMX Integration](#htmx-integration)
12. [Responsive Patterns](#responsive-patterns)
13. [Template Installation](#template-installation) (moved to the docs site)
14. [UsersService](#usersservice)
15. [API Reference](#api-reference)

---

## Quick Start, Core Concepts, App and ViewContext

These moved to the docs site, rewritten against the current API: [Getting started](https://panyam.github.io/goapplib/guide/getting-started/) builds a first app (its code is `docsite/examples/hello`, which CI compiles and renders), and [Concepts](https://panyam.github.io/goapplib/guide/concepts/) covers the app context, `Load` and loaders. In short, a page's `Load` takes `app *goapplib.App[AC]`, pages register as pointer types (`Register[*HomePage]`), and goapplib's own mixins can't go through `LoadAll` yet ([#107](https://github.com/panyam/goapplib/issues/107)). The sections below haven't moved yet and still show the older forms.

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
func (p *WithAuth) LoadWithAuth(r *http.Request, authMw *oneauth.Middleware, authSvc federatedauth.AuthUserStore) (error, bool) {
    p.LoggedInUserId = authMw.GetLoggedInUserId(r)
    p.IsLoggedIn = p.LoggedInUserId != ""

    if p.IsLoggedIn {
        // oneauth v0.1.x: stores take a context + request and return a response.
        resp, _ := authSvc.GetUserById(r.Context(), &accounts.GetUserByIDRequest{UserID: p.LoggedInUserId})
        if resp != nil && resp.User != nil {
            p.Username = resp.User.Profile()["username"].(string)
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
│   ├── BorderLayout.html
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

## BorderLayout

A 5-region layout component using pure CSS flexbox. Regions: North (top), South (bottom), East (right), West (left), Center (fills remaining space). All regions except Center are optional and collapse when empty. No JavaScript required.

```
┌──────────────────────────────┐
│           North              │  ← fixed height
├──────┬───────────────┬───────┤
│      │               │       │
│ West │    Center     │ East  │  ← Center fills remaining space
│      │               │       │
├──────┴───────────────┴───────┤
│           South              │  ← fixed height
└──────────────────────────────┘
```

**Parameters:**
- `.ContentId` — ID for center content div (default: `"border-layout-content"`)
- `.WrapperClass` — Additional CSS classes on wrapper
- `.CenterClass` — Additional CSS classes on center region
- `.FlexMode` — `"fill"` (default, flex-1), `"fixed"` (100% w/h), `"auto"` (natural size)

**Block overrides:** `BorderLayout_North`, `BorderLayout_South`, `BorderLayout_East`, `BorderLayout_West`

**Standard IDs:** `border-layout-wrapper`, `border-layout-center`, `border-layout-north`, `border-layout-south`, `border-layout-east`, `border-layout-west`

```html
{{# include "components/BorderLayout.html" #}}

{{ define "BorderLayout_North" }}
<div class="p-2 bg-gray-100 border-b flex items-center gap-2">
  <h1 class="text-lg font-bold">Editor</h1>
  <button class="btn-sm">Save</button>
</div>
{{ end }}

{{ define "BorderLayout_South" }}
<div class="p-1 text-xs text-gray-500 border-t">
  Status: Ready
</div>
{{ end }}

{{ template "BorderLayout" (dict "ContentId" "editor-canvas" "FlexMode" "fill") }}
```

---

## Island Pages

An island page is server-rendered, with a few client-side islands mounted into it. The server says which islands a page gets in a page spec; one tsappkit page class reads the spec and mounts each island from a registry. There's no hand-written `main.ts` finding elements by id.

### The Go side

Build a `page.Spec`, check it, and write it with the `PageSpecScript` partial (`templates/page/Islands.html`). The layout template places the slots as elements with `data-slot`:

```go
spec := page.Spec{Layout: "drawer", Islands: []page.Island{
    {Name: "player", Slot: "main", Presentation: "page", Config: map[string]any{"url": "/a.json"}},
    {Name: "chat", Slot: "side"},
}}
if err := spec.Validate(); err != nil { ... }
```

```html
<main data-slot="main"></main>
<aside data-slot="side"></aside>
{{ template "PageSpecScript" .Spec }}
```

An app that needs more in its spec embeds `page.Spec` in its own type (see the `page` package doc).

An island's `Load` says when the browser mounts it: `eager` (the default), `idle` once the page has settled, `visible` the first time its slot enters the viewport, or `media:<query>` when a media query matches (at once if it already does). `Validate` rejects anything else. Each island mounts once; nothing unmounts it when the query stops matching. Until then its slot shows whatever Go rendered there, so put a fallback in it. Don't defer an island that another island or the page needs at startup, since nothing waits for it. `IslandPage` (tsappkit 0.6.0 on) does the waiting, and a late island still goes through the component lifecycle; `mountIslands` takes a `defer` option for pages without `BasePage`.

### The browser side

Subclass `IslandPage` from `@panyam/tsappkit`. `registry()` names the islands this bundle can mount; `makeContext()` builds what they share, once, before the first island mounts:

```ts
import { IslandPage, type PageSpec } from "@panyam/tsappkit";

type Ctx = { api: ApiClient };
type Ext = { things: Thing[] };   // the fields the app's Go spec adds, if any

class HomePage extends IslandPage<Ctx, Ext> {
  protected registry() {
    return {
      player: (el, island, ctx, bus) => new PlayerIsland(el, island.config, ctx, bus),
      chat: (el, island, ctx, bus) => new ChatIsland(el, ctx, bus),
    };
  }
  protected readExtension(raw: Record<string, unknown>): Ext {
    return { things: Array.isArray(raw.things) ? raw.things.map(toThing) : [] };
  }
  protected makeContext(spec: PageSpec & Ext): Ctx {
    return { api: new ApiClient(spec.things) };
  }
}

IslandPage.loadAfterPageLoaded("homePage", HomePage, "HomePage");
```

Each factory returns an `LCMComponent`, so islands go through the usual lifecycle. An island the registry doesn't know, a slot that isn't on the page, or a factory that throws is logged with `console.warn` and skipped; the rest of the page still mounts. Give each esbuild entry its own registry so it only bundles the islands it can mount.

`readSpec` and `mountIslands` are exported too, for a page that mounts islands without `BasePage`.

### Slot fallbacks: one owner per region

Every region of the page has one owner. Either a Go template draws it, or an island does, and never both. The lesson came from lilbattle, where templates rendered the real content and then custom JS found those elements and "hydrated" them, so every change meant editing both and the two drifted apart.

What Go renders inside an island's `data-slot` is a placeholder for the time before the island mounts, which since lazy islands and load strategies can be quite a while. Make it fixed-size skeleton boxes so the layout doesn't jump when the island arrives, plus a `<noscript>` line if the island is all there is. It shouldn't be a second rendering of the island's content:

```html
<section data-slot="tools">
  <div class="skeleton h-64"></div>
  <noscript>The tool panel needs JavaScript.</noscript>
</section>
```

The island replaces the placeholder when it mounts. `SolidIsland` (`@panyam/tsappkit-solid`) clears its element on the first `activate`, in the same task that renders its tree, so the slot is never empty in between. Solid's own `render` appends, so a Solid tree mounted some other way needs to clear the element first. A hand-written island does the same with `el.replaceChildren(...)`. `make exercise-islands` checks that every mounted slot's fallback is gone.

### Checking and debugging islands

A spec that names an island the registry doesn't have only shows up as a console warning on whichever page uses it. `page.CheckIslands` finds those in a test, across all of an app's specs at once. Give it the names the registry can mount: `Assets.Names()` lists the lazy entries from the metafile, and islands bundled with the entry are added by hand:

```go
func TestSpecsNameKnownIslands(t *testing.T) {
	assets, err := page.LoadEsbuildMetafile("dist/meta.json", page.EsbuildOptions{OutDir: "dist", URLPrefix: "/static/"})
	if err != nil {
		t.Fatal(err)
	}
	known := append(assets.Names(), "toolbar") // toolbar is a plain factory, bundled with the entry
	if err := page.CheckIslands(known, homeSpec, gameSpec); err != nil {
		t.Fatal(err) // page specs name islands the registry doesn't have: "chat" (in home, game)
	}
}
```

In the browser, add `?islands` to a page's URL and `IslandPage` outlines each slot and labels it with its island, slot, load strategy and state: `hero · top · eager · mounted 212 ms`, `below · bottom · visible · waiting`, or a red `ghost · foot · eager · not in the registry`. Labels change as islands mount, and the times are from navigation start. Override `showIslandOverlay()` to tie it to your own debug setting. While it's on, labelled slots are `position: relative`, which can move an island's absolutely positioned content a bit. `mountIslands` reports the same things through its `onMount` and `onSkip` options, for pages without `BasePage`.

### Lazy islands and preload links

A registry entry wrapped in `lazy` loads its island's module only when the island mounts, so with esbuild's `--splitting` each island is its own chunk and a page downloads only the islands its spec names, each when its `Load` says:

```ts
import { IslandPage, lazy } from "@panyam/tsappkit";

protected registry() {
  return {
    player: lazy(() => import("./islands/player")), // default export is the factory
    chat: lazy(() => import("./islands/chat")),
  };
}
```

A lazy island mounts late even when it's eager, since its chunk arrives after the page has started, so the same rule applies: nothing that something else needs at startup. A chunk that fails to load is logged and skipped.

So that neither the eager islands' chunks nor the chunks the entry script imports wait for the entry to arrive before they're requested, Go writes `modulepreload` links for them. The entry's own chunks matter as soon as an island shares code with the page: esbuild moves that code (tsappkit's core, say) out of the entry into a chunk the entry imports, and the browser only finds it once the entry has been parsed. Build with a metafile (`esbuild ... --bundle --splitting --format=esm --metafile=dist/meta.json`), load it at startup, and write the links in `<head>`:

```go
assets, err := page.LoadEsbuildMetafile("dist/meta.json", page.EsbuildOptions{OutDir: "dist", URLPrefix: "/static/"})
```

```html
{{ template "IslandPreloads" (.Assets.For "main" .Spec) }}
```

`For` takes the name of the page's entry script (`web/main.ts` is `main`) and lists the chunks it imports, then the chunks of the spec's eager islands and everything they import. It leaves out the entry file itself, which the page's `<script>` loads, and `idle`, `visible` and `media` islands. Entries and islands are named by their source file's base name (`islands/player.ts` is `player`); `EsbuildOptions.Name` changes that. Another bundler's build can write the same shape `page.Assets` has (`{"entries": {"main": {"file": ..., "imports": [...]}}, "islands": {"player": {...}}}`) for `page.LoadAssets` to read.

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

Moved to [Getting started, Install](https://panyam.github.io/goapplib/guide/getting-started/#install). The two ways that work are reading goapplib's `templates/` from the module cache and vendoring them with templar.

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
    AuthService    federatedauth.AuthUserStore
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

## UsersService

The library provides a complete user management service with multiple storage backends.

### User Proto

Users are defined using Protocol Buffers with an extensible `extras` field:

```protobuf
message User {
  google.protobuf.Timestamp created_at = 1;
  google.protobuf.Timestamp updated_at = 2;
  string id = 3;
  string name = 4;
  string description = 5;
  repeated string tags = 6;
  string image_url = 7;
  string email = 8;
  google.protobuf.Struct extras = 20;  // App-specific data
}
```

### Service Interface

```go
type UsersService interface {
    CreateUser(ctx context.Context, req *v1.CreateUserRequest) (*v1.CreateUserResponse, error)
    GetUser(ctx context.Context, req *v1.GetUserRequest) (*v1.GetUserResponse, error)
    GetUsers(ctx context.Context, req *v1.GetUsersRequest) (*v1.GetUsersResponse, error)
    ListUsers(ctx context.Context, req *v1.ListUsersRequest) (*v1.ListUsersResponse, error)
    UpdateUser(ctx context.Context, req *v1.UpdateUserRequest) (*v1.UpdateUserResponse, error)
    DeleteUser(ctx context.Context, req *v1.DeleteUserRequest) (*v1.DeleteUserResponse, error)
    EnsureUser(ctx context.Context, userId, name, email, imageUrl string) (*v1.User, error)
}
```

### Storage Backends

#### FileSystem Backend (Development)

Stores users as JSON files. Ideal for development and testing.

```go
import fsgal "github.com/panyam/goapplib/services/backends/fs"

userService := fsgal.NewUsersService("./data/users")
```

#### GORM Backend (Production)

Works with PostgreSQL, MySQL, SQLite via GORM. Auto-migrates the schema.

```go
import gormgal "github.com/panyam/goapplib/services/backends/gorm"
import "gorm.io/driver/postgres"
import "gorm.io/gorm"

db, _ := gorm.Open(postgres.Open(dsn), &gorm.Config{})
userService := gormgal.NewUsersService(db)
```

#### Google Datastore Backend (GAE)

For Google Cloud Platform deployments. Supports namespacing for multi-tenancy.

```go
import gaegal "github.com/panyam/goapplib/services/backends/gae"
import "cloud.google.com/go/datastore"

client, _ := datastore.NewClient(ctx, projectID)
userService := gaegal.NewUsersService(client, "tenant-namespace")
```

### BaseUsersService Features

All backends inherit from `BaseUsersService` which provides:

- **Caching**: In-memory cache with configurable enablement
- **EnsureUser**: Creates user if not exists, returns existing otherwise
- **GetUser/GetUsers**: Single and batch user retrieval
- **ListUsers**: Paginated user listing

### AuthService Integration

Integrates with oneauth for OAuth authentication:

```go
import "github.com/panyam/goapplib/services"

// Create AuthService wrapping UsersService
authService := services.NewAuthService(authDB, usersService)

// In OAuth callback handler
user, err := authService.EnsureUser(ctx, identity.UserId, profile.Name, profile.Email, profile.ImageUrl)
```

### App-Specific Data

Use the `extras` field for application-specific user data:

```go
import "google.golang.org/protobuf/types/known/structpb"

extras, _ := structpb.NewStruct(map[string]any{
    "preferences": map[string]any{
        "theme": "dark",
        "notifications": true,
    },
    "subscription_tier": "pro",
})

user := &v1.User{
    Name:   "John Doe",
    Email:  "john@example.com",
    Extras: extras,
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
