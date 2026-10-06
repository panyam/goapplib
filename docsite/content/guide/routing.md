---
title: "Routing"
description: "Registering pages on a ServeMux, grouping them under a prefix, and the fluent MuxBuilder."
---

goapplib's routing is Go's own `http.ServeMux`, with helpers that turn a view type into a handler. Everything here works with Go 1.22's patterns (methods, `{id}` wildcards, `{$}`), and anything else you can do with a `ServeMux` still works, since that's all there is underneath. The code is the routes of `docsite/examples/games`, the app on the [Views and mixins]({{.Site.PathPrefix}}/guide/views/) page, built two ways:

```go
{{ includeFileText "examples/games/routes.go" }}
```

The example's test sends the same requests through both handlers and expects the same answers, so the two ways really are interchangeable.

## Register

`goapplib.Register[*GamePage](app, mux, "GET /{id}")` puts a handler for `GamePage` at the pattern. Each request gets a fresh `GamePage`, its `Load`, and then its template. Pass the pointer type, since `Load` has a pointer receiver.

The view's type names its template. `GamePage` renders `GamePage.html`, starting at the template defined as `GamePage`. Two options change what `Register` does:

- **`goapplib.WithTemplate("games/Detail")`** renders `games/Detail.html` from its `Detail` block instead, and `"games/Detail:Mobile"` names the block too.
- **`goapplib.WithMiddleware(mw...)`** wraps the page's handler, the first middleware outermost.

`Register` returns the mux, and makes a new one if you pass `nil`, which is handy in tests.

## Plain handlers

A route that isn't a page is a plain `http.Handler`, registered the usual way, which is mostly what you'd expect. The example's `DELETE /{id}` is one, and so is a static file server:

```go
mux.HandleFunc("DELETE /{id}", deleteGame(app))
mux.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("./static"))))
```

`goapplib.RegisterFunc` and `goapplib.RegisterHandler` do pretty much the same, and also make the mux if it's `nil`.

## Groups

A page group is a type whose `RegisterRoutes(app)` returns a `ServeMux` of routes relative to a prefix. `goapplib.RegisterGroup[*GamesGroup](app, mux, "/games")` mounts it at `/games/`, with `/games` stripped before the group's routes match, so the group's `"GET /{id}"` answers `/games/7`. A group can register other groups inside its own mux, which nests their prefixes. A request for `/games` without the slash gets redirected to `/games/` by the `ServeMux`.

Two things are easy to trip over inside a group.

- **`r.URL.Path` has the prefix stripped.** In the example's `NewGamePage`, `r.URL.Path` is `/new`, not `/games/new`. To send someone back to where they were, use `r.RequestURI`, which keeps the path the browser asked for. The example's login redirect does that, and its test checks for `next=/games/new`.
- **Patterns that could match the same path can't share a mux.** Go refuses, at registration, to put a nested group at `/admin/` beside a route `/{id}/view`, because `/admin/view` would match both. Rename one, or give the wildcard route a fixed first segment.

## MuxBuilder

`app.NewMux()` builds the same thing fluently. `Page(pattern, maker, opts...)` takes a function returning a fresh view (it renders exactly as `Register` would render that view type), `Group(prefix, setup)` nests routes under a prefix, and `Handler`, `HandleFunc` and `Static(pattern, dir)` add plain handlers. `Build()` returns the `ServeMux`.

`Use(mw)` wraps every route registered after it on that builder, groups included, and not the ones before it, so put it where it should start applying:

```go
mux := app.NewMux().
	Static("/static/", "./static").
	Use(requireLogin). // everything below needs a login
	Group("/games", setupGames).
	Build()
```

Before v0.7.1, `MuxBuilder.Page` panicked unless given `WithTemplate`, rendered a different block than `Register`, and dropped render errors, and `Use` did nothing ([#111](https://github.com/panyam/goapplib/issues/111)). We found that while writing these pages, so if you tried `MuxBuilder` before and gave up on it, that's why.
