# GoAppLib

## Version
0.7.0

## Provides
- web-app-scaffold: Server-rendered web application framework (stdlib-native)
- view-context: Generic ViewContext type system for page rendering
- page-mixins: Composable mixins (BasePage, WithPagination, WithFiltering, WithAuth, WithHtmx), chained with `LoadAll(r, w, app, loaders...)`. Since 0.7.0 `Loader` is non-generic (`Load(r, w, app any)`), which the mixins already satisfy, and an app's own steps are `LoaderFunc[AC]`, typed; before that no mixin was a `Loader[AC]` (#107)
- htmx-support: HTMX request detection and response utilities
- responsive-ui: Built-in UI components (drawers, modals, pagination, search filters)
- border-layout: 5-region layout component (North/South/East/West/Center) with pure CSS flexbox
- users-service: UsersService with multi-backend support (FS, GORM, Google Datastore)
- auth-integration: Integration with oneauth for authentication
- template-management: Template management via Templar integration
- rate-limiting: Rate limiting middleware for auth vs API endpoints
- admin-pages: Admin pages and user management
- page-spec: A page's layout and the islands it mounts (`page.Spec`), validated and written as a JSON script by `templates/page/Islands.html` for tsappkit's island page to mount from a registry; apps extend it by embedding (#30). `Island.Load` (`eager`, `idle`, `visible`, `media:<query>`) names when an island mounts; tsappkit 0.6.0's `IslandPage` waits for it (`parseLoad`, `scheduleMount`, `mountIslands`' `defer` option, #36), and `make exercise-islands` (mission #60) checks it
- island-chunks: tsappkit 0.6.0's `lazy(() => import("./islands/x"))` registry entries load each island as its own chunk when it mounts; `page.LoadEsbuildMetafile` (or `page.LoadAssets` for the common `{entries, islands: {name: {file, imports}}}` shape) gives `page.Assets`, whose `For(entry, spec)` lists the entry script's static chunks (#71, 0.6.3) and the eager islands' chunks for the `IslandPreloads` partial's `modulepreload` links (#35)
- slot-fallbacks: one owner per region; Go renders a placeholder in an island's `data-slot` and the island replaces it on mount. `SolidIsland` (tsappkit-solid 0.6.1) clears its element on the first `activate`, and `make exercise-islands` checks every mounted slot's fallback is gone (#39)
- island-dev-aids: `page.CheckIslands(known, specs...)` reports spec islands the registry doesn't have, with `Assets.Names()` for the lazy entries; `?islands` on an `IslandPage` URL turns on `IslandOverlay`, which labels each slot with its island's name, slot, load and state (waiting, mounted with its time, or why it won't mount); `mountIslands` takes `onMount` and `onSkip` (#42)
- island-page: `@panyam/tsappkit` 0.1.0 `IslandPage<Ctx, Ext>` reads the page spec from `#page-spec` and mounts each island into its `data-slot` from a registry (`name -> factory`), with `readSpec` (apps read their own spec fields through an extension) and `mountIslands`; a bad entry is logged and skipped (#27)
- wasm-worker-host: `wasmhost` runs an app's HTTP/Connect handlers as wasm in a Web Worker over files the page pushes in (`Serve`, `ServeRebuild` for handlers rebuilt on each mount, `Host.Do` in-process); `@panyam/tsappkit` 0.2.0 (`addFiles`, `workerMemory` from 0.3.0) `@panyam/tsappkit/wasmhost` gives the page `startWorker`, `workerFetch` (a Connect transport's fetch), `mountFiles`, `addFiles` (merge into a mount, `Host.Add`), `workerMemory` (the wasm's peak linear memory) and `filesFromDrop`. `make exercise-wasmhost` drives it in headless Chromium (#32)
- worker-state-cache: `wasmhost.Cache` (`Get`/`Put` by key, `ErrMiss`, `ErrNoCache`) with `CacheKey(parts...)` for a hash of a service's inputs, for state a wasm service can always rebuild (not a datastore); `BrowserCache()` over the Origin Private File System (installed by tsappkit 0.6.4's worker as `globalThis.wasmhostCache`, `wasmhost/<ns>/`), `DirCache(dir)` and `MemCache` natively. `make exercise-worker-state` (mission #74) checks a 32 MB state comes back after a reload (#76)
- one-shot-worker: `@panyam/tsappkit/wasmhost` 0.6.5's `oneShotFetch(opts)`, a fetch that runs each request in a fresh worker and terminates it once the response is in, so a job's peak wasm memory goes back to the browser; the job leaves its result in `wasmhost.Cache` for the long-lived worker (#77). `startWorker` now terminates a worker that fails to load
- worker-lanes: `@panyam/tsappkit/wasmhost` 0.6.6's `startLane(opts, { warm })`, one replaceable worker per kind of request; aborting a request on a lane terminates its worker (the only way to stop a Go job that never yields) and starts a new one, re-warmed from `wasmhost.Cache` (#80)
- worker-streaming: `wasmhost.Host.DoStream` hands each `Flush` to a callback while the handler runs, and the `http` export's chunk callback carries them to the page, so `workerFetch` (0.6.7) resolves at the first flush with a streamed body; aborting sends `cancel(id)`, which ends the handler's context. `oneShotFetch` and lanes hold their worker until a streamed body ends (#78)
- worker-watermark: `startLane(opts, { maxMemoryBytes, onRestart })` (0.6.8) measures the worker's wasm memory when the lane goes idle and, past the limit, replaces it with a fresh worker warmed from the cache, never interrupting a request (#79)

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
import "github.com/panyam/goapplib/page" // page.Spec, page.Island, page.ScriptJSON, page.Assets, page.CheckIslands
```

## Status
Active

## Conventions
- Generic ViewContext
- Stdlib-native (no custom router)
- Mixin-based composition
- BasePage mixin pattern
