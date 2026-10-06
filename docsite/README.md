# goapplib's documentation site

The site at https://panyam.github.io/goapplib/, built with [s3gen](https://github.com/panyam/s3gen).
This directory is its own Go module, so s3gen and its dependencies never reach goapplib's. Its
layout and tests follow jaala's docsite, which follows agni's.

## Commands

```sh
make -C docsite run       # serve at http://localhost:8085/goapplib/ (builds once; restart after an edit)
make -C docsite build     # bundle the demos and build into docsite/dist
make -C docsite test      # build, then the Go tests: templates parse, nav is wired, links resolve
make exercise-docsite     # mission #91's exercise: test, then run every page and demo in headless Chromium
```

`.github/workflows/docs.yml` runs the exercise on every PR and deploys `dist/` to Pages from `main`.
The exercise needs Chromium for playwright-core; locally it uses `~/.cache/ms-playwright`, and
`pnpm exec playwright-core install chromium` in this directory fetches it.

## Layout

- `content/` holds the pages, as Markdown with YAML front matter (`title`, `description`). A
  directory is a section, and its `index.md` is the section's landing page.
- `content/HeaderNavLinks.json` is the header nav, and `content/SiteMetadata.json` the site's name,
  description and links.
- `templates/` holds the page layout: `BasePage.html` with `Header`, `Sidebar`, `Content` and
  `Footer`, and one `nav/<Section>Nav.html` per section for its sidebar.
- `static/` is copied into the build as is: the stylesheet, the scripts for code blocks and the
  header dropdowns, and the demos' bundles (`static/demos/`, built, not checked in).
- `demos/` holds the live demos' sources, and `build.mjs` bundles them.
- `main.go` configures s3gen and the template functions pages can call; `demos.go` is `demo`.

## Adding a page

1. Write `content/<section>/<slug>.md` with a `title` and a `description`.
2. Link it from `content/<section>/index.md`.
3. Add it to `templates/nav/<Section>Nav.html`.
4. If the section has a header dropdown, add it to `content/HeaderNavLinks.json`.

A new section also needs its nav template included at the top of `templates/Sidebar.html` and a
`Contains $currentPath "/<section>/"` branch dispatching to it, plus an entry in the header nav.
`nav_test.go` checks each of these, so a missed edit fails `make test`.

Content goes through Go's text/template before Markdown, so a stray `{{` in a code sample blanks
the page (`template_test.go` catches it). In a Go sample, put a composite literal's inner brace on
its own line.

## Live demos

A demo is a small browser app in `demos/<name>/`: an `index.html` that loads `main.js`, and a
`main.ts` that `build.mjs` bundles with esbuild. A page embeds it with `{{ demo "<name>" }}`, or
`{{ demo "<name>" 320 }}` for a frame 320 px tall, on a line of its own. The build fails if the page
names a demo with no source.

Each demo runs in its own iframe, so it has its own document, globals and styles, and opens on its
own as a plain page. The frame is same-origin and shares the page's main thread, so it separates a
demo rather than sandboxing it; heavy work belongs in a worker, as it would in an app. It imports `demos/_lib/frame.ts`, which gives the frame the site's theme, and calls `ready()`
once it works or `failed(why)` if it doesn't. `run.mjs` waits up to 30 seconds for one of them on
every demo, and fails the run on an error or on silence. A demo can import goapplib's TS straight
from the repo (`../../../tsappkit/src/...`), the way the exercises do, so it shows the code on
`main` rather than the last npm release.

### Demos with Go in a worker

A demo with a `wasm/` directory (a `main` package with `//go:build js && wasm`) gets three more
files from `build.mjs`: its Go built as `app.wasm`, `wasm_exec.js` from the same Go toolchain, and
tsappkit's wasmhost worker as `worker.js`, bundled from `tsappkit/src/wasmhost/worker.ts`. All three
are versioned by content hash, and the demo reaches them through `assets` from `_lib/frame.ts`:
`startWorker({ ...assets, ns: "files" })`. The Go builds in this module, which requires goapplib
with `replace ../`, so a demo can import the exercises' services (`exercise/wasmhost/service`,
`exercise/workerstate/service`) and runs against the goapplib on `main`. A demo that imports the
exercises' generated Connect code gets `@bufbuild/*` and `@connectrpc/*` from this package, never
from the exercise's own `node_modules` (the one-copy plugin in `build.mjs`).

A frame sizes itself to its content (`_lib/frame.ts`), so the height a page gives `demo` is only the
first guess.

## The exercise

`run.mjs` serves `dist/` under `/goapplib/` the way Pages does, loads every page, fails on a page
error or a non-200, and runs every demo. Screenshots of each demo land in `dist-screenshots/`, and
CI keeps them as the `docsite-screenshots` artifact. It also checks for what mission #91 still
needs (the wasmhost and islands demos, the guide pages, the old guides retired). Those sit in
`pending` with their ticket: a pending check that fails is reported, and one that passes fails the
run (`XPASS`), so the PR that makes it pass takes it off the list.
