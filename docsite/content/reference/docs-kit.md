---
title: "tsappkit/docs"
description: "The pieces @panyam/tsappkit ships for an s3gen documentation site: copy buttons on code blocks, sidebar highlighting, an Ace action compiler, and the site's stylesheet."
---

`@panyam/tsappkit/docs` holds the browser side of a documentation site built with [s3gen](https://github.com/panyam/s3gen): the copy button on code blocks, the active link in the sidebar, and a compiler for code a reader types into an editor. The package also ships the site's stylesheet. tlex's and galore's docs use it. This site doesn't, since it keeps its own small scripts in `docsite/static/`.

It's a separate entry point from `@panyam/tsappkit`, so a page that imports only components doesn't bundle any of it.

```ts
import { initCodeBlocks, initPageSetup } from "@panyam/tsappkit/docs";

document.addEventListener("DOMContentLoaded", () => {
  initPageSetup();
  initCodeBlocks();
});
```

## Code blocks

`initCodeBlocks(selector = ".content-body pre")` wraps each matching `<pre>` in a `div.code-block` and adds a `button.code-block-copy`. The button copies the text of the block's `<code>`, or of the `<pre>` when it has no `<code>`. It shows a check mark for two seconds afterwards, with the `copied` class on the button. Where `navigator.clipboard` fails, it falls back to `document.execCommand("copy")`.

Calling it again skips blocks it already wrapped, so it's safe to run after new content arrives. A `<pre>` that holds another `<pre>` is skipped, and the inner one gets the button.

`new CodeBlock(pre)` does the same for one element.

## The sidebar

`highlightActiveSidebarLink(selector = ".sidebar .section-nav a")` adds the `active` class to each link whose `href` matches the current path, with or without a trailing slash. `initPageSetup({ sidebarSelector })` calls it, and is where shared page setup goes.

## ActionCompiler

`ActionCompiler` compiles JavaScript a reader types into an [Ace](https://ace.c9.io/) editor, as the body of a function of one argument, `node`. `ace-builds` is an optional peer dependency of `@panyam/tsappkit`, needed only for this.

```ts
import ace from "ace-builds";
import { ActionCompiler } from "@panyam/tsappkit/docs";

const editor = ace.edit("action");
const compiler = new ActionCompiler(editor, statusEl, (r) => {
  if (r.success) rerun();
});
const result = compiler.run(parseTree);
```

- The constructor recompiles 300 ms after the editor's last change, and calls the optional `onCompile` with each result.
- `compile()` compiles now, and returns `{ success, fn, error, line }`. An error is shown as an annotation in the editor, at the line it names, or at the first line if it names none. Empty code compiles to no function.
- `run(node)` calls the compiled function with `node` and returns `{ success, result, error, line }`. It fails when `node` is missing or the last compile failed. With no function, it succeeds with a `null` result. A runtime error is annotated at its line when the stack has one.
- `statusEl`, if given, shows ✓, "✗ Compile Error" or "✗ Runtime Error", with a `success` or `error` class.
- `getCompileError()`, `hasCompiledAction()` and `clear()` report and reset its state.

The code runs through `new Function` on the page itself, with the page's privileges, so use it only for code the reader typed in their own browser.

## The stylesheet

The SCSS is in `@panyam/tsappkit/styles/docs/`, with `DocsPage.scss` as the entry and one partial per region (`_variables`, `_base`, `_header`, `_layout`, `_content`, `_footer`, `_responsive`). The package ships it compiled as `dist/docs/DocsPage.css` and `DocsPage.min.css`. It styles the class names an s3gen docs layout uses: `.site-header`, `.page-container`, `.sidebar`, `.section-nav`, `.main-content`, `.content-body`, `.code-block`, `.site-footer` and their parts.

Load it with a `<link>` in the layout's `<head>` rather than importing it from a script. A script import applies the styles only after the bundle runs, so the page shows unstyled first. tlex and galore copy it into their static files as part of the build:

```makefile
copycss:
	cp node_modules/@panyam/tsappkit/dist/docs/DocsPage.css static/css/tsappkit-docs.css
```

```html
<link rel="stylesheet" href="/static/css/tsappkit-docs.css">
<link rel="stylesheet" href="/static/css/main.css">
```

The layout's `href` adds the site's path prefix, as the rest of its static links do. A site's own stylesheet goes after it and overrides the custom properties in `_variables.scss`: colors (`--color-primary`, `--color-bg`, `--color-text` and the rest), `--header-height`, `--sidebar-width`, `--content-max-width`, and the font and spacing scales. Dark mode reads `:root.dark`, the class `ThemeManager` sets.

```css
:root {
  --color-primary: #7c3aed;
  --color-bg: #fafafa;
}
:root.dark {
  --color-primary: #a78bfa;
  --color-bg: #0f0f1a;
}
```
