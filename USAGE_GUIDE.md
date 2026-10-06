# goapplib - Go Web Application Library

A lightweight, stdlib-native Go library for building server-rendered web applications with:
- **Composable mixins** for common page behaviors (pagination, auth, filtering)
- **Template hierarchy** with Templar for inheritance and composition
- **HTMX-ready** components for progressive enhancement
- **Responsive patterns** for mobile/desktop layouts

## Table of Contents

1. [Quick Start, Core Concepts, App and ViewContext](#quick-start-core-concepts-app-and-viewcontext) (moved to the docs site)
4. [Views, Mixins, Route Registration, Page Groups, MuxBuilder](#views-mixins-route-registration-page-groups-muxbuilder) (moved to the docs site)
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

These moved to the docs site, rewritten against the current API: [Getting started](https://panyam.github.io/goapplib/guide/getting-started/) builds a first app (its code is `docsite/examples/hello`, which CI compiles and renders), and [Concepts](https://panyam.github.io/goapplib/guide/concepts/) covers the app context, `Load` and loaders. In short, a page's `Load` takes `app *goapplib.App[AC]`, pages register as pointer types (`Register[*HomePage]`), and since 0.7.0 goapplib's own mixins chain through `LoadAll` ([#107](https://github.com/panyam/goapplib/issues/107)). The sections below haven't moved yet and still show the older forms.

---

## Views, Mixins, Route Registration, Page Groups, MuxBuilder

These moved to the docs site, rewritten against the current API and built around a compiled, tested example (`docsite/examples/games`): [Views and mixins](https://panyam.github.io/goapplib/guide/views/) covers `Load`, early returns, each mixin's fields and helpers, `AuthProvider` and your own mixins, and [Routing](https://panyam.github.io/goapplib/guide/routing/) covers `Register`, its options, page groups and `MuxBuilder`. The old sections here showed APIs that don't exist (`WithPagination.ToProto`, `SetFromResponse`, a three-argument `LoadWithAuth`) and a `MuxBuilder` that panicked before v0.7.1.

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

Moved: [Views and mixins](https://panyam.github.io/goapplib/guide/views/) and [Routing](https://panyam.github.io/goapplib/guide/routing/) walk through `docsite/examples/games`, a list, detail and login-checked app with a page group, which CI compiles and drives over HTTP.

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
func LoadAll(r *http.Request, w http.ResponseWriter, app any, loaders ...Loader) (error, bool)
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
