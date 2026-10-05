# goapplib - Web Application Library

A lightweight, stdlib-native Go library for building server-rendered web applications.

## Module Structure

```
goapplib/
├── go.mod              # Module definition
├── USAGE_GUIDE.md      # Comprehensive documentation
├── SUMMARY.md          # This file
│
├── app.go              # App type, template setup, default funcs
├── view.go             # View, Loader, PageGroup interfaces
├── mixins.go           # BasePage, WithPagination, WithFiltering, WithAuth, WithHtmx
├── htmx.go             # HtmxResponse helpers
├── register.go         # Register, RegisterGroup, RegisterFunc, RegisterHandler
├── muxbuilder.go       # Fluent MuxBuilder API
├── ratelimit.go        # Rate limiting middleware
│
├── protos/             # Protocol Buffer definitions
│   └── goapplib/v1/
│       ├── users.proto          # User message and UsersService RPCs
│       ├── gorm/users.proto     # GORM database mapping
│       └── datastore/users.proto # Google Datastore mapping
│
├── gen/                # Generated protobuf code
│   ├── go/goapplib/v1/ # Go protobuf messages
│   ├── gorm/           # GORM models and converters
│   └── datastore/      # Datastore entities and converters
│
├── services/           # Service interfaces and implementations
│   ├── users_service.go         # UsersService interface + BaseUsersService
│   ├── auth_service.go          # AuthService (deprecated - pass-through to oneauth)
│   ├── oneauth_bridge.go        # UserBridge adapts v1.User to accounts.User
│   └── backends/
│       ├── fs/users_service.go      # FileSystem backend
│       ├── gorm/users_service.go    # GORM/PostgreSQL backend
│       └── gae/users_service.go     # Google Datastore backend
│
├── page/               # page.Spec (a page's islands, #30), page.Assets (chunks to preload, #35), CheckIslands (#42)
├── wasmhost/           # Run an app's HTTP/Connect handlers as wasm in a Web Worker (#32)
├── exercise/wasmhost/  # `make exercise-wasmhost`: the worker host end to end in headless Chromium
├── exercise/islands/   # `make exercise-islands`: lazy islands by load strategy in headless Chromium (#60)
├── scripts/            # npm-publish.sh (publishes the TS packages from a tag, #46), check-versions.sh (#66)
│
├── tsappkit/           # @panyam/tsappkit: BasePage and component lifecycle, IslandPage, wasmhost client
├── tsappkit-solid/     # @panyam/tsappkit-solid: SolidIsland, mounting a Solid tree as an island
│
└── templates/          # Base templates (copy/symlink to your app)
    ├── BasePage.html
    ├── Header.html
    ├── page/Islands.html   # PageSpecScript (a page.Spec as #page-spec JSON), IslandPreloads
    └── components/
        ├── BorderLayout.html
        ├── Drawer.html
        ├── EntityGrid.html
        ├── EntityTable.html
        ├── Modal.html
        ├── Pagination.html
        ├── SearchFilter.html
        ├── SplashScreen.html
        └── Toast.html
```

## Key Features

### 1. Generic ViewContext
Your application defines its own ViewContext type. The library uses Go generics to work with any ViewContext.

### 2. stdlib-Native
Everything uses `*http.ServeMux` and `http.Handler`. No custom router.

### 3. Composable Mixins
Embed common behaviors in your pages:
- `BasePage` - Page metadata (title, body class, etc.)
- `WithPagination` - Pagination state and helpers
- `WithFiltering` - Search query, sort, view mode
- `WithAuth` - Authentication state
- `WithHtmx` - HTMX request detection

### 4. Chain Loading
```go
goal.LoadAll(r, w, vc, &p.BasePage, &p.WithPagination, &p.WithAuth)
```

### 5. Simple Registration
```go
goal.Register[HomePage](app, mux, "/")
goal.RegisterGroup[GamesGroup](app, mux, "/games")
```

### 6. HTMX-Ready
- `WithHtmx` mixin for request detection
- `HtmxResponse` helpers for response headers
- HTMX-aware template components

### 7. Responsive Components
Templates include mobile-friendly patterns:
- **BorderLayout** — 5-region layout (North/South/East/West/Center) using pure CSS flexbox with configurable flex modes
- Drawers with swipe gestures
- Bottom navigation bar
- Responsive grids

### 8. Rate Limiting
Built-in sliding window rate limiting middleware:
- Separate limits for auth endpoints (stricter) vs API endpoints
- Configurable limits and time windows
- Custom key functions for rate limit identity

```go
// Use default config (10 auth/15min, 100 API/min)
rateLimiter := goal.NewRateLimitMiddleware(nil)

// Apply to routes
mux.Handle("/auth/", rateLimiter.WrapAuth(authHandler))
mux.Handle("/api/", rateLimiter.WrapAPI(apiHandler))
```

### 9. UsersService with Multi-Backend Support
Complete user management with pluggable storage backends:
- **Interface-based design**: `UsersService` interface + `BaseUsersService` with caching
- **Storage abstraction**: `UserStorageProvider` interface for backend implementations
- **Multiple backends**: FileSystem (dev), GORM/PostgreSQL (production), Google Datastore (GAE)
- **Proto-defined**: User message with extensible `extras` field for app-specific data
- **Generated DAL**: Uses protoc-gen-dal for type-safe database access

```go
// FileSystem backend (development)
import fsgal "github.com/panyam/goapplib/services/backends/fs"
userService := fsgal.NewUsersService("./data/users")

// GORM backend (PostgreSQL, MySQL, SQLite)
import gormgal "github.com/panyam/goapplib/services/backends/gorm"
userService := gormgal.NewUsersService(db)

// Google Datastore backend (GAE)
import gaegal "github.com/panyam/goapplib/services/backends/gae"
userService := gaegal.NewUsersService(client, "namespace")

// Use the service
resp, err := userService.CreateUser(ctx, &v1.CreateUserRequest{
    User: &v1.User{
        Name:  "John Doe",
        Email: "john@example.com",
    },
})
```

### 10. Island Pages

A server-rendered page names its client-side islands in a `page.Spec`, written into the page as `#page-spec` JSON. tsappkit's `IslandPage` reads it and mounts each island into its `data-slot` from a registry of factories, so apps don't hand-write a `main.ts` per page. Apps extend the spec by embedding `page.Spec` in Go and reading their own fields with `readExtension` in TS. Each island's `Load` (`eager`, `idle`, `visible`, `media:<query>`) says when it mounts; `IslandPage` (tsappkit 0.6.0) waits for it and the slot shows Go's fallback until then, which the island replaces (one owner per region). A `lazy(() => import(...))` registry entry makes an island its own chunk, and `page.Assets` (read from esbuild's metafile) gives the page `modulepreload` links for its entry's chunks and its eager islands' (`For(entry, spec)`). `page.CheckIslands` catches spec islands the registry doesn't have, and `?islands` on a page labels each slot with its island's state. `page/testdata/spec.json` is checked by both the Go and TS tests so the format can't drift. See USAGE_GUIDE.md, "Island Pages".

### 11. Wasm Worker Host

`wasmhost.Serve` runs an app's HTTP/Connect handlers as wasm inside a Web Worker, over files the page pushes in before it asks anything (`mountFiles` replaces a mount, `addFiles` merges into one). The page's generated Connect clients talk to it through `workerFetch` (`@panyam/tsappkit/wasmhost`), so they don't know whether a server or the worker answered. `ServeRebuild` rebuilds the handler on every mount change for apps whose handler snapshots its files, `Host.Do` serves a request in-process for native tests, and `workerMemory` reports the wasm's peak memory. The mounts are goutils' `mountfs` over `memfs`. The design reasoning is in docs/PRESENTER_CONTRACT_THESIS.md. State a service builds for itself and could always rebuild (a parsed design, a model) goes in a `wasmhost.Cache`, keyed by a hash of its inputs (`wasmhost.CacheKey`): `BrowserCache` keeps it in the page origin's private file system, so it outlives a reload, and `DirCache` and `MemCache` do the same natively. It's a cache rather than a datastore, so a miss or a lost cache only means a rebuild. A job with a big transient peak (an ingest that parses a large input on the way to a small result) goes through `oneShotFetch`, which runs it in a fresh worker and ends that worker afterwards, since ending a worker is the only way to give wasm memory back; the job leaves its result in the cache, which every worker of the namespace shares, and the long-lived worker loads that. Cache calls wait on the browser, so they're made from a handler, never a `js.FuncOf` callback.

### 12. AuthService Integration (Deprecated)
**Deprecated**: AuthService is now a thin pass-through to oneauth. For new code, use oneauth directly.

The AuthService wrapper is maintained for backwards compatibility but delegates entirely to oneauth:
- `EnsureAuthUser` → `federatedauth.NewEnsureAuthUserFunc` (with channel linking support)
- `CreateLocalUser` → `localauth.NewCreateUserFunc`
- `ValidateLocalCredentials` → `localauth.NewCredentialsValidator`
- Now supports `UsernameStore` for username-based login

Since **oneauth v0.1.x**, the underlying `accounts.UserStore` / `IdentityStore` / `ChannelStore` interfaces use a context + request/response shape. The AuthService pass-throughs mirror that — e.g. `CreateUser(ctx, *accounts.CreateUserRequest) (*accounts.CreateUserResponse, error)`.

```go
// Legacy usage (still works, but deprecated)
import "github.com/panyam/goapplib/services"
authService := services.NewAuthService("/path/to/storage")
user, err := authService.EnsureAuthUser("oauth", "google", token, userInfo)

// Recommended: Use oneauth directly (v0.1.x sub-module imports)
import (
    "github.com/panyam/oneauth/federatedauth"
    "github.com/panyam/oneauth/stores/gorm"
)
config := federatedauth.EnsureAuthUserConfig{
    UserStore:     gorm.NewUserStore(db),
    IdentityStore: gorm.NewIdentityStore(db),
    ChannelStore:  gorm.NewChannelStore(db),
    UsernameStore: gorm.NewUsernameStore(db), // Optional
}
ensureUser := federatedauth.NewEnsureAuthUserFunc(config)
user, err := ensureUser("oauth", "google", token, userInfo)
```

## Quick Start

See `USAGE_GUIDE.md` for complete documentation.

```go
// 1. Define ViewContext
type ViewContext struct {
    ClientMgr *services.ClientMgr
}

// 2. Define Page
type HomePage struct {
    goal.BasePage
}

func (p *HomePage) Load(r *http.Request, w http.ResponseWriter, vc *ViewContext) (error, bool) {
    p.Title = "Home"
    return nil, false
}

// 3. Setup App
vc := &ViewContext{ClientMgr: services.NewClientMgr()}
templates := goal.SetupTemplates("./templates")
app := goal.NewApp(vc, templates)

// 4. Register Routes
mux := http.NewServeMux()
goal.Register[HomePage](app, mux, "/")

// 5. Serve
http.ListenAndServe(":8080", mux)
```

## Template Installation

Since Go modules can't export static files, copy or symlink the templates directory:

```bash
# Option 1: Symlink
ln -s $(go list -m -f '{{.Dir}}' github.com/panyam/goapplib)/templates ./templates/goapplib

# Option 2: Copy
cp -r $(go list -m -f '{{.Dir}}' github.com/panyam/goapplib)/templates ./templates/goapplib
```

Then configure template loader with fallback:
```go
templates := goal.SetupTemplates(
    "./templates",                    // Your overrides
    "./templates/goapplib",             // Library defaults
)
```

## Next Steps

- [x] Publish to github.com/panyam/goapplib
- [x] Add middleware utilities (rate limiting)
- [x] Add UsersService with multi-backend support (FS, GORM, GAE)
- [x] Add AuthService integration with oneauth
- [ ] Add more component templates
- [ ] Add form helpers
