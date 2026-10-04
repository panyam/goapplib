# GoAppLib

## Version
0.6.1

## Provides
- web-app-scaffold: Server-rendered web application framework (stdlib-native)
- view-context: Generic ViewContext type system for page rendering
- page-mixins: Composable mixins (WithPagination, WithFiltering, WithAuth, WithHtmx)
- htmx-support: HTMX request detection and response utilities
- responsive-ui: Built-in UI components (drawers, modals, pagination, search filters)
- border-layout: 5-region layout component (North/South/East/West/Center) with pure CSS flexbox
- users-service: UsersService with multi-backend support (FS, GORM, Google Datastore)
- auth-integration: Integration with oneauth for authentication
- template-management: Template management via Templar integration
- rate-limiting: Rate limiting middleware for auth vs API endpoints
- admin-pages: Admin pages and user management
- page-spec: A page's layout and the islands it mounts (`page.Spec`), validated and written as a JSON script by `templates/page/Islands.html` for tsappkit's island page to mount from a registry; apps extend it by embedding (#30). `Island.Load` (`eager`, `idle`, `visible`, `media:<query>`) names when an island mounts; tsappkit 0.6.0's `IslandPage` waits for it (`parseLoad`, `scheduleMount`, `mountIslands`' `defer` option, #36), and `make exercise-islands` (mission #60) checks it
- island-chunks: tsappkit 0.6.0's `lazy(() => import("./islands/x"))` registry entries load each island as its own chunk when it mounts; `page.LoadEsbuildMetafile` (or `page.LoadAssets` for the common `{islands: {name: {file, imports}}}` shape) gives `page.Assets`, whose `For(spec)` lists the eager islands' chunks for the `IslandPreloads` partial's `modulepreload` links (#35)
- slot-fallbacks: one owner per region; Go renders a placeholder in an island's `data-slot` and the island replaces it on mount. `SolidIsland` (tsappkit-solid 0.6.1) clears its element on the first `activate`, and `make exercise-islands` checks every mounted slot's fallback is gone (#39)
- island-page: `@panyam/tsappkit` 0.1.0 `IslandPage<Ctx, Ext>` reads the page spec from `#page-spec` and mounts each island into its `data-slot` from a registry (`name -> factory`), with `readSpec` (apps read their own spec fields through an extension) and `mountIslands`; a bad entry is logged and skipped (#27)
- wasm-worker-host: `wasmhost` runs an app's HTTP/Connect handlers as wasm in a Web Worker over files the page pushes in (`Serve`, `ServeRebuild` for handlers rebuilt on each mount, `Host.Do` in-process); `@panyam/tsappkit` 0.2.0 (`addFiles`, `workerMemory` from 0.3.0) `@panyam/tsappkit/wasmhost` gives the page `startWorker`, `workerFetch` (a Connect transport's fetch), `mountFiles`, `addFiles` (merge into a mount, `Host.Add`), `workerMemory` (the wasm's peak linear memory) and `filesFromDrop`. `make exercise-wasmhost` drives it in headless Chromium (#32)

## Module
github.com/panyam/goapplib

## Location
newstack/goapplib/main

## Stack Dependencies
- goutils v0.1.14 (github.com/panyam/goutils) — `memfs` and `mountfs` back wasmhost's mounts
- oneauth v0.1.13 (github.com/panyam/oneauth) — uses accounts + federatedauth + localauth + stores/fs subpackages. v0.1.x stores use a context + request/response API (CreateUser(ctx, *CreateUserRequest) etc.)
- protoc-gen-dal (github.com/panyam/protoc-gen-dal)
- templar v0.1.0 (github.com/panyam/templar) — NewFileSystemLoader now takes FSFolder values; use templar.LocalFolder(path) for local directories

## Integration

### Go Module
```go
// go.mod
require github.com/panyam/goapplib 0.0.26

// Local development
replace github.com/panyam/goapplib => ~/newstack/goapplib/main
```

### Key Imports
```go
import "github.com/panyam/goapplib/views"
import "github.com/panyam/goapplib/page" // page.Spec, page.Island, page.ScriptJSON, page.Assets
```

## Status
Active

## Conventions
- Generic ViewContext
- Stdlib-native (no custom router)
- Mixin-based composition
- BasePage mixin pattern
