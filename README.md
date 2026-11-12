# goapplib

[![Go Reference](https://pkg.go.dev/badge/github.com/panyam/goapplib.svg)](https://pkg.go.dev/github.com/panyam/goapplib)
[![Tests](https://github.com/panyam/goapplib/actions/workflows/test.yml/badge.svg)](https://github.com/panyam/goapplib/actions/workflows/test.yml)
[![Docs](https://github.com/panyam/goapplib/actions/workflows/docs.yml/badge.svg)](https://panyam.github.io/goapplib/)
[![Go Report Card](https://goreportcard.com/badge/github.com/panyam/goapplib)](https://goreportcard.com/report/github.com/panyam/goapplib)
[![Release](https://img.shields.io/github/v/tag/panyam/goapplib?sort=semver&label=release)](https://github.com/panyam/goapplib/tags)
[![License](https://img.shields.io/github/license/panyam/goapplib)](LICENSE)

[![tsappkit](https://img.shields.io/npm/v/@panyam/tsappkit?label=%40panyam%2Ftsappkit)](https://www.npmjs.com/package/@panyam/tsappkit)
[![tsappkit-solid](https://img.shields.io/npm/v/@panyam/tsappkit-solid?label=%40panyam%2Ftsappkit-solid)](https://www.npmjs.com/package/@panyam/tsappkit-solid)
[![tsappkit-dock](https://img.shields.io/npm/v/@panyam/tsappkit-dock?label=%40panyam%2Ftsappkit-dock)](https://www.npmjs.com/package/@panyam/tsappkit-dock)

A Go library for server-rendered web apps. Go renders each page through templates. The parts that need client code are islands, and a small TypeScript kit mounts them. An app's own Go handlers can also run in the browser as wasm, for when a round trip to the server is too slow.

**Docs: https://panyam.github.io/goapplib/** has a getting-started guide, a guide for each part with live demos, and the Go API read from `main`. [Demos](https://panyam.github.io/goapplib/demos/) lists every live demo in one place.

## What's in it

| Package | What it does | Guide |
| --- | --- | --- |
| `github.com/panyam/goapplib` | Pages are Go views that load their own data. Mixins add pagination, auth, filtering and htmx awareness. The same [templar](https://github.com/panyam/templar) templates render whole pages and htmx fragments. Everything runs on a plain `http.ServeMux`. | [Concepts](https://panyam.github.io/goapplib/guide/concepts/) |
| [`@panyam/tsappkit`](tsappkit/) | Gives a page a component lifecycle. `IslandPage` mounts each island the Go page declares: at once, when the browser is idle, when its slot scrolls into view, or when a media query matches. | [Island pages](https://panyam.github.io/goapplib/guide/islands/) |
| [`@panyam/tsappkit-solid`](tsappkit-solid/) | `SolidIsland` mounts a Solid tree as an island. | [Island pages](https://panyam.github.io/goapplib/guide/islands/) |
| [`@panyam/tsappkit-dock`](tsappkit-dock/) | An IDE-like dock on dockview-core. Panels sit in a grid, float, or slide out of drawers on any edge. The saved layout picks up new panels. | [Docks and drawers](https://panyam.github.io/goapplib/guide/dock/) |
| [`wasmhost`](wasmhost/) | Runs an app's HTTP or Connect handlers as wasm in a Web Worker. The page's clients can't tell it from a server. | [Go handlers in a Web Worker](https://panyam.github.io/goapplib/guide/wasmhost/) |

All four share one version, in `VERSION`. The [Overview](https://panyam.github.io/goapplib/overview/) says more about each.

## Install

```sh
go get github.com/panyam/goapplib@v0.7.11
```

```sh
npm install @panyam/tsappkit            # islands and the component lifecycle
npm install @panyam/tsappkit-solid      # Solid islands
npm install @panyam/tsappkit-dock       # the dock
```

goapplib's layouts are templates, so your app needs them on disk at run time. [Getting started](https://panyam.github.io/goapplib/guide/getting-started/#install) shows two ways to get them there: read them from the module cache, or vendor them with templar.

## A page

A page is a Go type with a `Load` method. Its type name picks its template.

```go
import goal "github.com/panyam/goapplib"

type HomePage struct {
	goal.BasePage
	Header          Header
	NavigationItems []NavItem
	Tagline         string
}

func (p *HomePage) Load(r *http.Request, w http.ResponseWriter, app *goal.App[*Site]) (error, bool) {
	p.Title = "Home"
	p.Header.AppName = app.Context.Name
	p.Tagline = app.Context.Tagline
	return nil, false
}

func NewHandler(site *Site, appTemplates, goapplibTemplates string) http.Handler {
	app := goal.NewApp(site, goal.SetupTemplates(appTemplates, goapplibTemplates))
	mux := http.NewServeMux()
	goal.Register[*HomePage](app, mux, "/{$}")
	return mux
}
```

The full example lives in [`docsite/examples/hello`](docsite/examples/hello), and CI renders it on every pull request. [Getting started](https://panyam.github.io/goapplib/guide/getting-started/) walks through it, template included.

## Guides

- Building pages: [Concepts](https://panyam.github.io/goapplib/guide/concepts/), [Views and mixins](https://panyam.github.io/goapplib/guide/views/), [Routing](https://panyam.github.io/goapplib/guide/routing/), [Templates and components](https://panyam.github.io/goapplib/guide/templates/), [htmx](https://panyam.github.io/goapplib/guide/htmx/)
- Client code: [Island pages](https://panyam.github.io/goapplib/guide/islands/), [Components and the page lifecycle](https://panyam.github.io/goapplib/guide/components/), [Guided tours](https://panyam.github.io/goapplib/guide/tour/)
- Docks: [Docks and drawers](https://panyam.github.io/goapplib/guide/dock/), [Dock experiments](https://panyam.github.io/goapplib/guide/dock-lab/)
- wasmhost: [Go handlers in a Web Worker](https://panyam.github.io/goapplib/guide/wasmhost/), [State, heavy jobs and long requests](https://panyam.github.io/goapplib/guide/wasmhost-state/)
- Reference: [Go API](https://panyam.github.io/goapplib/reference/api/), [UsersService](https://panyam.github.io/goapplib/reference/users/), [tsappkit/docs](https://panyam.github.io/goapplib/reference/docs-kit/)

## Who uses it

[thambura](https://github.com/panyam/thambura), [agni](https://github.com/panyam/agni), diffpp and [lilbattle](https://github.com/turnforge/lilbattle) run on goapplib.

## Developing

```sh
make setup   # git hooks
make test    # Go tests, wasmhost tests as wasm under Node, and each TS package's tests
make exercise-islands   # one browser exercise in headless Chromium; CI runs all of them
```

[SUMMARY.md](SUMMARY.md) tours the repo, and [NEXTSTEPS.md](NEXTSTEPS.md) records where each line of work stopped. The docs site builds from [`docsite/`](docsite/).

## License

[Apache 2.0](LICENSE)
