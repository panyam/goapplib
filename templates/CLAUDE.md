# templates/

goapplib's base templates, rendered through templar.

- `BasePage.html` reads `.Header.AppName`, `.Header.IsLoggedIn`, `.Header.Username` and `.NavigationItems` (each `{Href, Label, Active}`) from every page, and goapplib defines no types for them, so a page without them fails with `can't evaluate field`. A page overrides `BodySection` through `namespace` and `extend` (see `docsite/examples/hello`); defining it again fails with `multiple definition of template`.
- Template actions inside an HTML comment are live: `<!-- {{ define "X" }} -->` is a real definition. Put examples in templar's `{{#/* ... */#}}` comment. `page/Islands.html` is parsed by plain `html/template`, so it can't use that syntax.
