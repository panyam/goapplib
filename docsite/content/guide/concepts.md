---
title: "Concepts"
description: "How a request flows through a goapplib app: the mux, the page, its Load, and the template it renders."
---

goapplib is a fairly thin layer over Go's standard library. A page is a Go type, routing is an `http.ServeMux`, and rendering is Go's `html/template` through [templar](https://github.com/panyam/templar), which adds includes, namespaces and template inheritance. There's no framework runtime between your handler and `net/http`, so anything that works with a `ServeMux` (middleware, `httptest`, other handlers on the same mux) works with goapplib.

## What it's built on

- **The standard library.** Pages register on an `*http.ServeMux` and each one is an `http.Handler`.
- **Mixins.** A page embeds the behaviors it needs (`BasePage`, `WithPagination`, `WithFiltering`, `WithAuth`, `WithHtmx`) and gets their fields.
- **Template inheritance.** goapplib's layouts are templar templates, and a page's template extends them rather than copying them.
- **Progressive enhancement.** Pages are plain HTML from the server. htmx can swap fragments of them, and islands of client code can mount into them, but neither is required.
- **Nothing implicit.** Every route is registered by hand, and a page's data comes from its own `Load`.

## A request, start to finish

```mermaid
flowchart LR
  req[GET /games] --> mux[http.ServeMux]
  mux --> h[handler from Register]
  h --> new[new GamesPage]
  new --> load["Load(r, w, app)"]
  load -->|error| e500[500]
  load -->|"finished = true"| done[response already written]
  load -->|ok| render["RenderTemplate: GamesPage.html, block GamesPage"]
  render --> out[HTML]
```

`goapplib.Register[*GamesPage](app, mux, "/games")` puts a handler on the mux. For each request, that handler makes a fresh `GamesPage`, so pages never share state between requests, and calls its `Load`. If `Load` returns an error, the response is a 500 with the error's text. A `true` second value means `Load` has written the response itself (a redirect, a JSON answer), so nothing renders. Otherwise the handler renders the template named after the type, with the page as its data.

## The app and its context

`goapplib.App[AC]` is created once at startup and shared by every page:

```go
templates := goapplib.SetupTemplates("./templates", goapplibTemplates)
app := goapplib.NewApp(site, templates)
```

`AC` is your own type, the app context, and `app.Context` is the value you passed in. It's where the things every page needs go: service clients, configuration, an auth provider. A page reaches them in `Load` as `app.Context.Whatever`, which keeps pages free of globals and makes them easy to test with a context built for the test.

`app.Templates` is the templar template group. `SetupTemplates` builds one that looks in each folder you give it, in order, so your own templates come first and override goapplib's, and adds goapplib's template functions (`goapplib.DefaultFuncMap()`). To render some other way, set `app.RenderTemplateFunc`.

## Loaders

A page usually builds its data in steps, some from goapplib's mixins and some its own. `goapplib.LoadAll(r, w, app, loaders...)` runs them in order and stops at the first that fails or finishes, returning what it returned. Each step is a `goapplib.Loader`:

```go
type Loader interface {
	Load(r *http.Request, w http.ResponseWriter, app any) (err error, finished bool)
}
```

goapplib's mixins (`BasePage`, `WithPagination`, `WithFiltering`, `WithAuth`, `WithHtmx`) are loaders as they are, since none of them needs the app. An app's own step usually does, so it's a `goapplib.LoaderFunc[AC]`, which gets the app typed as `*App[AC]`. Here's a page from the getting-started example that chains both:

```go
{{ includeFileText "examples/hello/list.go" }}
```

`goapplib.AuthLoader[*Site](&p.WithAuth, provider)` is a `LoaderFunc` too, for auth, and its app type has to be written out, since Go can't infer it. A `LoaderFunc` handed an app of some other type doesn't run, and `LoadAll` returns an error naming both types, which renders as a 500.

This shape is new in v0.7.0. Before it, `Loader` was generic over the app context (`Loader[AC]`, with `app *App[AC]`), which none of goapplib's own mixins satisfied, so `LoadAll` couldn't chain them, though the old guides showed it doing so. We found that while writing these pages against a compiled example ([#107](https://github.com/panyam/goapplib/issues/107)).

## Where next

[Getting started]({{.Site.PathPrefix}}/guide/getting-started/) builds a one-page app. [Island pages]({{.Site.PathPrefix}}/guide/islands/) adds client code to a page, and [Go handlers in a Web Worker]({{.Site.PathPrefix}}/guide/wasmhost/) runs an app's Go in the browser.
