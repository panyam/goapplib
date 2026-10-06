---
title: "Getting started"
description: "A first goapplib app: one page, rendered on the server through goapplib's layout."
---

This page builds a small app with one page, rendered by Go through goapplib's layout. The code is `docsite/examples/hello` in goapplib's repo, included here as it is, and a test renders its page on every pull request. So if you copy it from here, it builds against the goapplib on `main`. (We started doing it this way after finding that the old guides' examples didn't compile.)

## Install

```sh
go get github.com/panyam/goapplib@v0.6.9
```

goapplib's layouts (`BasePage.html`, the header, the components) are templates, not Go, so your app needs them on disk at run time. There are two ways to get them there, and either is fine to start with.

**Read them from the module cache.** Go already downloaded them with the module, and `go list` says where:

```sh
{{`go list -m -f '{{.Dir}}' github.com/panyam/goapplib`}}
```

Pass that directory's `templates/` folder to `goapplib.SetupTemplates` after your own, which is what the example does. It's the quickest way to start, and mostly what you'd want for development.

**Vendor them with templar.** [templar](https://github.com/panyam/templar), the template library under goapplib, can fetch a pinned copy of goapplib's templates into your repo, which is better for a deploy that has no module cache. Add a `templar.yaml`:

```yaml
sources:
  goapplib:
    url: github.com/panyam/goapplib
    path: templates
    ref: v0.6.9

vendor_dir: ./templar_modules
search_paths:
  - ./templates
  - ./templar_modules
```

Then run `templar get` (`go install github.com/panyam/templar/cmd/templar@latest` installs it), load the config with `tmplr.NewSourceLoaderFromConfig`, and refer to goapplib's files as `@goapplib/BasePage.html`. Give `NewSourceLoaderFromConfig` an absolute path (`filepath.Abs` it first), since a relative one can fail depending on the working directory. templar's [vendoring guide](https://github.com/panyam/templar/blob/main/docs/vendoring.md) has the rest.

## The app

The app is one Go package and one template. Here's the Go:

```go
{{ includeFileText "examples/hello/app.go" }}
```

A few things in it are easy to get wrong.

- **The app context is your type.** `goal.App[*Site]` carries a `*Site`, built once at startup, and every page's `Load` gets it as `app.Context`. It's the place for the services and settings that pages share.
- **`Load` runs once per request**, on a fresh page value, before rendering. It returns `(error, bool)`: an error renders as a 500, and `true` means `Load` already wrote the response itself (a redirect, say), so nothing renders.
- **Register the pointer type.** `goal.Register[*HomePage]`, not `Register[HomePage]`, which doesn't compile, since `Load` has a pointer receiver. goapplib makes a fresh `HomePage` for each request from that pointer type.
- **The type's name picks the template.** `HomePage` renders `HomePage.html`, starting at the template defined as `HomePage`. `goal.WithTemplate("pages/Other:Block")` changes both (file `pages/Other.html`, starting at `Block`), and the block name defaults to the file's base name.
- **Embed `goal.BasePage`.** It carries the fields goapplib's layout reads (`Title`, `MetaTitle`, `BodyClass` and the rest). Without it, the page fails to render with `can't evaluate field MetaTitle`.
- **The layout reads `.Header` and `.NavigationItems` too.** goapplib's header template reads `.Header.AppName`, `.Header.IsLoggedIn` and `.Header.Username`, and its navigation reads `.NavigationItems`, a list of `{Href, Label, Active}`. goapplib doesn't define types for these yet, so the page brings its own, as above.

The template extends goapplib's `BasePage` and fills in its body:

```html
{{ includeFileText "examples/hello/templates/HomePage.html" }}
```

`namespace` loads goapplib's `BasePage.html` under the name `Goal`, and `extend` makes a copy of its `BasePage` called `HomeLayout`, with `BodySection` swapped for this page's `HomeBody`. You can't just define `BodySection` again in the page's own file, because Go's templates refuse a second definition (`multiple definition of template "BodySection"`). `BasePage` has more sections to swap the same way: `TitleSection`, `ExtraHeadSection`, `HeaderSection`, `FooterSection`, `PostScriptsSection` and others, listed at the top of goapplib's `templates/BasePage.html`.

And a `main` to serve it:

```go
{{ includeFileText "examples/hello/serve/main.go" }}
```

Run it from `examples/hello`, with goapplib's templates from the module cache:

```sh
{{`go run ./serve -goapplib-templates "$(go list -m -f '{{.Dir}}' github.com/panyam/goapplib)/templates"`}}
```

`http://localhost:8080/` shows the page, with goapplib's header and your body. It's unstyled, because goapplib's layout loads `/static/css/tailwind.css`, which is the app's to build and serve (`mux.Handle("/static/", http.StripPrefix("/static", http.FileServer(http.Dir("./static"))))`). htmx comes from a CDN, so it works as is. templar logs a `Template not found` warning for each file it looks for in your folder before finding it in goapplib's. That's expected, and it's a bit noisy.

## Testing a page

The example's test renders the page through `httptest` and checks what came out, which is a quick way to catch a template that no longer matches its page:

```go
{{ includeFileText "examples/hello/hello_test.go" }}
```

Check the body as well as the status. A template that fails partway through still answers 200, with the page cut off where it failed ([#108](https://github.com/panyam/goapplib/issues/108)).

## Deploying to App Engine

Nothing in goapplib is specific to App Engine, but the old integration guide deployed there, so here's the short version. Read the port from `PORT`, serve `static/` through App Engine's own handler, and send everything else to the app:

```yaml
runtime: go124

handlers:
- url: /static
  static_dir: static
- url: /.*
  script: auto
```

Pick the `runtime` that matches your `go.mod`, and ship goapplib's templates with the app, since App Engine has no module cache at run time (vendor them with templar, as above).

## Where next

[Concepts]({{.Site.PathPrefix}}/guide/concepts/) covers how a request flows through goapplib, what the app context is for, and how loaders chain. For apps built with it, see lilbattle's `web/` (a game site with auth and pagination) and excaliframe's `site/` (a small marketing site).
