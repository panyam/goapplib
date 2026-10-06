---
title: "Templates and components"
description: "How a page's template extends goapplib's layout, the template functions, and the components that ship with goapplib."
---

goapplib's templates are Go's `html/template`, loaded through [templar](https://github.com/panyam/templar), which adds three directives: `include` to pull in another file, `namespace` to load one under a prefix, and `extend` to copy a template with some of its blocks swapped. Every page here is from `docsite/examples/listing` in goapplib's repo, a list of projects with search, paging and an htmx delete, plus an editor page in a `BorderLayout`. Its tests render each page on every pull request, so the templates on this page are ones that work.

## Extending BasePage

A page's template doesn't copy goapplib's layout. It extends it, swapping in its own body:

```html
{{ includeFileText "examples/listing/templates/ProjectsPage.html" }}
```

`namespace "Goal" "BasePage.html"` loads goapplib's `BasePage.html` with every template in it prefixed `Goal:`. `extend "Goal:BasePage" "ProjectsLayout" "Goal:BodySection" "ProjectsBody"` makes `ProjectsLayout`, a copy of `BasePage` that uses `ProjectsBody` wherever it used `BodySection`. The page's own template (`ProjectsPage`, named after the Go type) then renders `ProjectsLayout`.

It has to be done this way, which is a bit roundabout the first time. Including `BasePage.html` and defining `BodySection` again fails with `multiple definition of template "BodySection"`, because templar parses a file and what it includes as one template set. That's also why the `Results` block can sit in the same file: it's a separate template, rendered inside the body for the whole page, and on its own for an htmx search (see the [htmx]({{.Site.PathPrefix}}/guide/htmx/) page).

`BasePage` has these blocks to swap: `TitleSection`, `CSSSection`, `HTMXSection`, `ExtraHeadSection`, `SplashScreenSection`, `HeaderSection`, `BodySection`, `ModalSection`, `ToastSection`, `FooterSection`, `AppContainerSection`, `AppScriptSection`, `PreScriptsSection` and `PostScriptsSection`. Extend swaps several at once by listing more pairs. It reads `.Header` and `.NavigationItems` from the page as well as `BasePage`'s fields, and `CustomHeader: true` skips goapplib's header, which the editor page below does to put its own toolbar there.

## Template functions

`goapplib.SetupTemplates` adds `goapplib.DefaultFuncMap()`:

- `safeHTML`, `safeHTMLAttr`, `safeJS` and `safeURL` mark a value as trusted, so `html/template` doesn't escape it.
- `default` falls back when a value is nil or an empty string, with the fallback first (`{{`{{ default "entity-grid" .ContainerId }}`}}`, or `{{`{{ .ContainerId | default "entity-grid" }}`}}`). `dict` and `list` build a map or a slice in a template (`(dict "ContentId" "canvas")`).
- `add` and `sub` are integer arithmetic, which goapplib's own `Pagination` and `SearchFilter` need. They arrived in v0.7.2, and before that those two components failed to render with only goapplib's functions.
- `eq`, `ne`, `or`, `and` and `not` replace Go's built-in ones. `eq` and `ne` compare with Go's `==` on whatever they're given, so an `int` and an `int64` holding 1 aren't equal, and `or` and `and` treat an empty string or a zero value as false.
- `dset` and `lset` set a map key or a slice element and return it. `ToJson` writes a value as JSON for a `<script>`, and `Indented` turns a block of text's newlines into `<br/>` (its first argument, a number of spaces, isn't used).

## Components

goapplib's components live in `templates/components/`. They're styled with Tailwind classes, so an app that uses them builds Tailwind over goapplib's templates as well as its own. Each one reads the fields listed here from what you pass it.

- **`EntityListing`** draws a whole listing (a header, search and sort controls, a grid or table of items, and an empty state) from a `goapplib.EntityListingData[T]`. `NewEntityListingData[Project]("Projects", "/projects/%s")` sets up the defaults, and `WithCreate(url, label)`, `WithEdit(format)`, `WithView(format)`, `WithDelete(format)` and `WithHtmx(searchUrl)` turn on the rest. Each item needs `Id`, `Name` and `Description`. Load it with a namespace, as the example does, since it defines its own `EmptyState`, and so does `EntityGrid`.
- **`Pagination`** takes the page itself, since it reads `CurrentPage`, `HasPrevPage`, `HasNextPage`, `PrevPage`, `NextPage`, `Pages`, `TotalCount` and `TotalPages` from `WithPagination`, and `Query` and `Sort` from `WithFiltering`, to keep them in its links. It draws nothing when there's only one page. `PaginationHtmx` does the same with htmx, and also reads `BaseUrl` and `Target` from what you pass it.
- **`SearchFilter`** is a search box, a sort menu and a view toggle. It reads `Query`, `Placeholder`, `SearchId`, `SortOptions`, `SortId`, `ViewModes`, `CreateUrl` and `CreateLabel`, and with `HtmxEnabled`, `SearchUrl`, `Target` and `Indicator`. `SearchFilterSimple` is a plain form version.
- **`EntityGrid`** and **`EntityTable`** are the grid and the table on their own. The grid reads `Items` (each with `Id`, `Name`, `Description`, `PreviewUrl` and `UpdatedAt`) and has blocks (`GridItem`, `GridItemTitle`, `GridItemDescription`, `GridItemImage`, `GridItemMeta`, `GridItemActions`) to override. The table reads `Columns` (each with `Key`, `Label` and `Sortable`) and `Items`, with blocks `TableHeaders`, `TableRow`, `TableCells` and `TableActions`.
- **`Modal`** and **`ConfirmModal`**, and **`Toast`**, are what `BasePage`'s `ModalContainer` and `ToastContainer` hold. A modal reads `Id`, `Title`, `MaxWidth`, `ShowClose` and `ShowFooter`, with blocks `ModalBody` and `ModalFooter`. A toast reads `Type`, `Title`, `Message`, `Dismissible`, `AutoDismiss` and `DismissAfter`.
- **`Drawer`**, **`DrawerRight`** and **`BottomBar`** are for small screens: a drawer from the bottom (`Id`, `Title`, `Height`, `Position`, with a `DrawerContent` block) or the right (`Id`, `Title`, `Width`), and a bar of `Items` (each `Label`, `Icon`, `Href` or `Action` or `DrawerId`, and `Active`).

## BorderLayout

`BorderLayout` fills its parent with a center and up to four regions around it, in plain CSS flexbox. The editor page uses it for a toolbar and a status bar:

```html
{{ includeFileText "examples/listing/templates/EditorPage.html" }}
```

The regions are blocks named `BorderLayout_North`, `BorderLayout_South`, `BorderLayout_East` and `BorderLayout_West`, empty unless you swap them in with `extend`, as above. It reads `ContentId` (the id of the center's content, `border-layout-content` by default), `WrapperClass`, `CenterClass` and `FlexMode`, which is `fill` (take the rest of the parent, the default), `fixed` (100% width and height) or `auto` (as big as the content). The wrapper's id is `border-layout-wrapper` and the center's is `border-layout-center`. The regions get no element or id of their own, so give whatever you put in one an id if a script needs it.

We found, while writing this page, that until v0.7.2 every `BorderLayout` without its own North rendered "My Toolbar", because an example in the template's comment was a live `define` (Go's templates parse what's inside an HTML comment). Upgrading fixes it, if you saw that in an app.

## Different layouts for different screens

Most of the time, and mostly by design, Tailwind's breakpoints (`sm:`, `md:`, `lg:`) are enough, with `Drawer` and `BottomBar` for what only small screens show. Register the same page more than once when a layout differs enough to want a different template, with `WithTemplate`, at different routes, or pick in a small handler that serves the right registration:

```go
mobile := goapplib.Register[*EditorPage](app, nil, "/editor", goapplib.WithTemplate("EditorPageMobile"))
desktop := goapplib.Register[*EditorPage](app, nil, "/editor")
mux.HandleFunc("/editor", func(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("layout"); err == nil && c.Value == "mobile" {
		mobile.ServeHTTP(w, r)
		return
	}
	desktop.ServeHTTP(w, r)
})
```
