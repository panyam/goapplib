---
title: "htmx"
description: "Serving a page or just a fragment of it from the same route, the HtmxResponse helpers, and out-of-band updates."
---

[htmx](https://htmx.org) lets a server-rendered page swap parts of itself from the server's responses, without writing client code. goapplib's `BasePage` loads htmx (from a CDN, in its `HTMXSection` block), and gives a page what it needs to answer htmx. That's pretty much three things: knowing an htmx request when it sees one, rendering only the part htmx wants, and setting the response headers htmx reads. The examples are from `docsite/examples/listing`, whose tests send each request with and without htmx's headers.

## One route, a page or a fragment

The listing's search box asks for `/projects/?q=…` through htmx and swaps the results into the page. A browser opening the same URL wants the whole page. One registration serves both:

```go
goal.Register[*ProjectsPage](app, mux, "GET /projects/{$}", goal.WithFragmentTemplate("ProjectsPage:Results"))
```

`WithFragmentTemplate("ProjectsPage:Results")` means: for an htmx request that wants a fragment, render the `Results` block of `ProjectsPage.html` instead of the page. The page decides whether it does, through `WithHtmx`, which it embeds and loads like any other mixin. `WithHtmx` reads htmx's request headers into `IsHtmx`, `IsBoosted`, `Target`, `Trigger`, `TriggerName`, `CurrentURL` and `Prompt`, and its `ShouldRenderFragment()` is true for an htmx request that isn't a boosted link. A boosted link (`hx-boost`) is htmx following a normal link, which wants the whole page. A view that doesn't embed `WithHtmx`, or a request without htmx's headers, gets the full page.

The template keeps the fragment in the page, so the two can't drift apart. The page's body renders `Results`, and htmx gets `Results` alone:

```html
{{`{{ define "Results" }}`}}
<div id="results">…the listing and the pager…</div>
{{`{{ end }}`}}

{{`{{ define "ProjectsBody" }}`}}<main>{{`{{ template "Results" . }}`}}</main>{{`{{ end }}`}}
```

`WithFragmentTemplate` works with `MuxBuilder.Page` too. `goapplib.SmartRegister[V](app, mux, pattern, fullSpec, fragmentSpec)` is the older spelling of the same thing (`Register` with `WithTemplate(fullSpec)` and `WithFragmentTemplate(fragmentSpec)`), kept for the code that calls it. We folded it into `Register` in v0.7.2, since it was a third copy of the page handler.

## Answering an action

The listing's delete button sends `DELETE /projects/{id}` through htmx. A plain handler answers it, differently for htmx and for a browser without JavaScript:

```go
if !goal.IsHtmxRequest(r) {
	http.Redirect(w, r, "/projects/", http.StatusSeeOther)
	return
}
goal.NewHtmxResponse(w).Trigger("entityUpdated")
app.RenderTemplate(w, "ProjectsPage", "DeleteResponse", map[string]any{"Count": left})
```

`goapplib.NewHtmxResponse(w)` sets the response headers htmx acts on, and each method returns it, so they chain:

- `Trigger(event)`, `TriggerWithData(event, data)`, `TriggerAfterSwap(event)` and `TriggerAfterSettle(event)` fire events on the page (`HX-Trigger` and friends). `EntityListing`'s grid listens for `entityUpdated` (or its `RefreshTrigger`) and reloads from its `RefreshUrl`, when it has one.
- `Redirect(url)` sends the browser somewhere else, `Location(url)` and `LocationWithContext(spec)` load a page through htmx without a full reload, and `Refresh()` reloads.
- `PushURL(url)` and `ReplaceURL(url)` change the address bar.
- `Retarget(selector)`, `Reswap(style)` and `Reselect(selector)` change where and how the response is swapped in.
- `StopPolling()` answers with status 286, which tells a polling element to stop. It writes the status at once, so call it after the other headers.

Set the headers before writing the body, as with any header. On the request side, `goapplib.IsHtmxRequest(r)`, `IsBoostedRequest`, `HtmxTarget`, `HtmxTrigger`, `HtmxCurrentURL` and `HtmxPrompt` read htmx's headers for a handler that isn't a page.

## Out-of-band updates

A response can update more than the element htmx asked about. Anything in it with `hx-swap-oob="true"` replaces the element on the page with the same id. After a delete, the listing's count changes too, so the delete response is just the count, out of band:

```html
{{`{{ define "DeleteResponse" }}`}}<p id="project-count" hx-swap-oob="true">{{`{{ .Count }}`}} projects</p>{{`{{ end }}`}}
```

`app.RenderTemplate(w, "ProjectsPage", "DeleteResponse", data)` renders one block of a page's template with whatever data you give it, which suits small responses like this one. It writes nothing when rendering fails, so the handler can still answer with `http.Error`, though headers it set first (the `HX-Trigger` here) go out with the error.
