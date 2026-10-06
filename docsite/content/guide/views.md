---
title: "Views and mixins"
description: "A page's Load, its early returns, the mixins that read the request for it, and how LoadAll chains them."
---

A view is a page: a Go type with a `Load` method that fills in its fields from the request, which its template then renders. This page walks through the views of a small app, `docsite/examples/games` in goapplib's repo: a paginated, searchable list of games, a page per game, and a "new game" page behind a login. Its test drives it over HTTP on every pull request, so the code here is what CI built.

## The view contract

```go
type View[AC any] interface {
	Load(r *http.Request, w http.ResponseWriter, app *App[AC]) (err error, finished bool)
}
```

goapplib makes a fresh view for each request, calls `Load`, and then does one of three things:

- **`err != nil`:** answers 500 with the error's text, and nothing renders.
- **`finished == true`:** does nothing more, since `Load` wrote the response itself (a redirect, a 404, some JSON).
- **otherwise:** renders the view's template with the view as its data.

`finished` wins over `err`, so a `Load` that wrote a response returns `nil, true`.

## The example's views

Here are all three, with the app context and the auth stand-in:

```go
{{ includeFileText "examples/games/pages.go" }}
```

A few things in it are worth a closer look.

- **`GamesListPage` is built from steps.** `LoadAll` runs goapplib's mixins, which read the request (`?page=`, `?pageSize=`, `?q=`, `?sort=`), then the page's own `loadGames`, which needs the app, so it's a typed `LoaderFunc[*Site]`. `LoadAll` stops at the first step that fails or finishes. The [Concepts]({{.Site.PathPrefix}}/guide/concepts/#loaders) page has the details.
- **`GamePage` answers 404 itself.** An unknown id calls `http.NotFound` and returns `nil, true`, so nothing renders. Returning an error instead would answer 500, which is the wrong status for a missing game.
- **`NewGamePage` redirects.** It loads `WithAuth` through `AuthLoader`, and if nobody's logged in, it redirects to the login page and returns `nil, true`.
- **`Chrome` holds what every page shares.** goapplib's layout reads `.Header` and `.NavigationItems` from every page ([Getting started]({{.Site.PathPrefix}}/guide/getting-started/#the-app) explains why), so each page embeds one `Chrome` rather than declaring them again. A struct you embed in every page is the simplest kind of mixin you can write.

## goapplib's mixins

Each mixin is a struct you embed in a page. Its fields become the page's fields (so the template reads `.CurrentPage`, not `.WithPagination.CurrentPage`), and its `Load` fills them from the request. Every one of them is a `Loader`, so `LoadAll` chains them.

**`BasePage`** carries what goapplib's layout needs: `Title`, `MetaTitle`, `MetaDescription`, `CanonicalUrl`, `BodyClass`, `ActiveTab`, `CustomHeader`, `DisableSplashScreen`, `SplashTitle`, `SplashMessage` and `BodyDataAttributes`. Its `Load` only sets a default `BodyClass`, so a page sets the rest itself, usually `Title`.

**`WithPagination`** reads `?page=` (counting from 0) and `?pageSize=` (20 by default, at most 100) into `CurrentPage` and `PageSize`. `SetTotal(total, hasMore)`, called once the page knows how many items there are, fills in `TotalCount`, `HasPrevPage`, `HasNextPage` and `Pages`, the page numbers to show (up to five, around the current one). `Offset()`, `PrevPage()` and `NextPage()` do the arithmetic, and `Paginator()` returns the struct, for templates that take it as a whole.

**`WithFiltering`** reads `?q=`, `?sort=` and `?view=` into `Query`, `Sort` and `ViewMode`. An empty sort becomes `"modified_desc"` and an empty view becomes `"table"`.

**`WithHtmx`** reads htmx's request headers into `IsHtmx`, `IsBoosted`, `Target`, `Trigger`, `TriggerName`, `CurrentURL` and `Prompt`. `ShouldRenderFragment()` is true for an htmx request that isn't a boosted link, which is when a page usually wants to render only a fragment. Templates and htmx have their own page (coming with [#96](https://github.com/panyam/goapplib/issues/96)).

**`WithAuth`** holds `LoggedInUserId`, `Username`, `IsLoggedIn` and `IsOwner`. Its own `Load` does nothing, since it needs your auth service to do anything useful, which is a bit of a trap if you chain it like the other mixins. Load it with `AuthLoader[*YourApp](&p.WithAuth, provider)`, or call `p.WithAuth.LoadWithAuth(r, provider)` yourself. `provider` is a `goapplib.AuthProvider`, a two-method interface:

```go
type AuthProvider interface {
	GetLoggedInUserId(r *http.Request) string // "" when nobody is logged in
	GetUserById(id string) (AuthUser, error)  // AuthUser has Profile() map[string]any
}
```

`LoadWithAuth` sets `Username` from the profile's `"username"`, if it has one. `IsOwner` is left for the page to set, since only the page knows what's being owned. The example's `CookieAuth` is a stand-in that trusts a cookie, so don't copy it into a real app.

## Your own mixins

We think of a mixin as nothing more than a struct with fields and, if it reads the request, a `Load`. To chain it through `LoadAll`, give it goapplib's loader shape, `Load(r *http.Request, w http.ResponseWriter, app any) (error, bool)`. If it needs the app, assert it there, or make the step a `LoaderFunc[*YourApp]` instead:

```go
{{ includeFileText "examples/games/withgame.go" }}
```

Then a page embeds `WithGame` and puts `&p.WithGame` in its `LoadAll`.
