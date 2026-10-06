---
title: "Island pages"
description: "A server-rendered page with a few client-side islands, each mounted when its load strategy says, from a spec Go writes into the page."
---

An island page is a page Go renders on the server, with a few client-side islands mounted into its slots. Go decides which islands a page gets and writes that into the page as a page spec, and one tsappkit page class reads the spec and mounts each island from a registry. There's no hand-written `main.ts` finding elements by id.

Both demos on this page were rendered by Go, with the same `page.Spec` and partials an app's server uses. GitHub Pages can't run a server, so the site's build runs that Go once and serves what it wrote.

## The Go side

Build a `page.Spec`, check it, and write it into the page with the `PageSpecScript` partial from `templates/page/Islands.html`. The layout template places the slots, as elements with `data-slot`:

```go
spec := page.Spec{Layout: "drawer", Islands: []page.Island{
	{Name: "player", Slot: "main", Presentation: "page", Config: map[string]any{"url": "/a.json"}},
	{Name: "chat", Slot: "side"},
}}
if err := spec.Validate(); err != nil {
	return err
}
```

```html
<main data-slot="main"></main>
<aside data-slot="side"></aside>
{{`{{ template "PageSpecScript" .Spec }}`}}
```

`Validate` rejects a spec with no layout, an island with no name, a slot name that isn't lowercase letters, digits and dashes, a `Load` it doesn't know, or two islands in one slot. `Config` goes to the island's factory as is, so it has to be JSON. An app that needs more in its spec (the things a page starts with, say) embeds `page.Spec` in its own type; the `page` package doc shows how.

## The browser side

Subclass `IslandPage` from `@panyam/tsappkit`. `registry()` names the islands this bundle can mount, and `makeContext()` builds what they share, once, before the first island mounts:

```ts
import { IslandPage, lazy, type PageSpec } from "@panyam/tsappkit";

class HomePage extends IslandPage<Ctx> {
  protected registry() {
    return {
      player: lazy(() => import("./islands/player")), // default export is the factory
      chat: (el, island, ctx, bus) => new ChatIsland(el, ctx, bus),
    };
  }
  protected makeContext(spec: PageSpec): Ctx {
    return { api: new ApiClient() };
  }
}

IslandPage.loadAfterPageLoaded("homePage", HomePage, "HomePage");
```

Each factory gets the slot's element, the island's spec, the context and the page's event bus, and returns an `LCMComponent`, so islands go through the usual component lifecycle. An island the registry doesn't know, a slot that isn't on the page, or a factory that throws is logged with `console.warn` and skipped, and the rest of the page still mounts. Give each esbuild entry its own registry, so it only bundles the islands it can mount. An app whose Go spec adds fields reads them in `readExtension`, and they arrive typed on the spec `makeContext` gets.

## When each island mounts

An island's `Load` says when the browser mounts it:

- **`eager`** (the default) mounts at once.
- **`idle`** mounts once the browser is idle.
- **`visible`** mounts the first time any of its slot scrolls into view.
- **`media:<query>`** mounts when the media query matches, at once if it already does.

Each island mounts once, and nothing unmounts it when its query stops matching. Until it mounts, its slot shows whatever Go rendered there, so put a placeholder in it (see the next section).

In the demo below, each of the four islands is a separate chunk, and the table shows when its code arrived and when it mounted. `below` sits at the bottom of a scrolling box, so its code doesn't download until you scroll to it. `narrow` waits for a frame under 600 px wide. Inside an iframe, a media query follows the iframe's width rather than the window's, so the button at the bottom shrinks the frame and `narrow` mounts right then. (On a phone it has already mounted.)

{{ demo "islands-load" 560 }}

Don't defer an island that another island or the page needs at startup, since nothing waits for it. A deferred island mounts after the page's own startup has finished, so it gets its own run through the lifecycle, and nothing orders its `activate` after the page's. A `lazy` island counts as deferred even when it's eager, because its chunk arrives after the page has started.

## Placeholders, and one owner per region

Every region of the page has one owner. Either a Go template draws it or an island does, never both. We learned this one from lilbattle, where templates rendered the real content and then custom JS found those elements and "hydrated" them, so every change meant editing both, and the two drifted apart.

What Go renders inside an island's slot is a placeholder for the time before the island mounts, which, with lazy islands and load strategies, can be quite a while. Make it fixed-size skeleton boxes, so the layout doesn't jump when the island arrives, plus a `<noscript>` line if the island is all there is. It shouldn't be a second rendering of the island's content:

```html
<section data-slot="tools">
  <div class="skeleton h-64"></div>
  <noscript>The tool panel needs JavaScript.</noscript>
</section>
```

The island replaces the placeholder when it mounts. `SolidIsland`, from `@panyam/tsappkit-solid`, clears its element on its first `activate`, in the same task that renders its tree, so the slot is never empty in between. Solid's own `render` appends rather than replacing, so a Solid tree mounted some other way has to clear the element first, and a hand-written island does it with `el.replaceChildren(...)`.

The demo below is a `SolidIsland` over two skeleton bars. It loads a bit too fast to see them, so "Reload with a 1.5 s delay" holds its chunk back for a moment:

{{ demo "islands-solid" 260 }}

```tsx
import { createSignal } from "solid-js";
import { SolidIsland } from "@panyam/tsappkit-solid";

export default function counter(el: HTMLElement, island: IslandSpec, ctx: unknown, bus: EventBus) {
  return new SolidIsland("counter", el, () => {
    const [clicks, setClicks] = createSignal(0);
    return <button onClick={() => setClicks(clicks() + 1)}>Clicked {clicks()} times</button>;
  }, bus);
}
```

## Lazy islands and preload links

A registry entry wrapped in `lazy` loads its island's module only when the island mounts. With esbuild's `--splitting`, each island is its own chunk, so a page downloads only the islands its spec names, each when its `Load` says. A chunk that fails to load is logged and skipped.

That can leave the browser waiting on the entry script before it even asks for the chunks it needs at once. An eager island's chunk isn't requested until the entry has run. Neither is the code esbuild moved out of the entry into a shared chunk, which is mostly tsappkit's core once an island shares it. So Go writes `modulepreload` links for both. Build with a metafile, load it at startup, and write the links in `<head>`:

```go
// esbuild web/main.ts --bundle --splitting --format=esm --outdir=dist --metafile=dist/meta.json
assets, err := page.LoadEsbuildMetafile("dist/meta.json", page.EsbuildOptions{OutDir: "dist", URLPrefix: "/static/"})
```

```html
{{`{{ template "IslandPreloads" (.Assets.For "main" .Spec) }}`}}
```

`For` takes the name of the page's entry script (`web/main.ts` is `main`) and lists the chunks it imports, then the chunks of the spec's eager islands and everything they import. It leaves out the entry itself, which the page's `<script>` loads, and the `idle`, `visible` and `media` islands, since preloading those would download what the page means to put off. Entries and islands are named by their source file's base name (`islands/player.ts` is `player`), and `EsbuildOptions.Name` changes that. The "Preloaded" column in the first demo comes from these links. Another bundler's build can write the same shape `page.Assets` has for `page.LoadAssets` to read.

## Checking and debugging

A spec that names an island the registry doesn't have shows up only as a console warning, and only on a page that uses it, which is pretty easy to miss. `page.CheckIslands` finds those in a test, across all of an app's specs at once. Give it the names the registry can mount. `Assets.Names()` lists the lazy ones from the metafile, and islands bundled with the entry are added by hand:

```go
func TestSpecsNameKnownIslands(t *testing.T) {
	assets, err := page.LoadEsbuildMetafile("dist/meta.json", page.EsbuildOptions{OutDir: "dist", URLPrefix: "/static/"})
	if err != nil {
		t.Fatal(err)
	}
	known := append(assets.Names(), "toolbar") // toolbar is a plain factory, bundled with the entry
	if err := page.CheckIslands(known, homeSpec, gameSpec); err != nil {
		t.Fatal(err)
	}
}
```

In the browser, add `?islands` to a page's URL and `IslandPage` outlines each slot and labels it with its island, slot, load strategy and state, like `hero · top · eager · mounted 212 ms`, `below · bottom · visible · waiting`, or a red `ghost · foot · eager · not in the registry`. The labels change as islands mount, and the times are from navigation start. The second demo's "Show the ?islands overlay" button turns it on. Override `showIslandOverlay()` to tie it to your own debug setting. While it's on, labelled slots are `position: relative`, which can move an island's absolutely positioned content a bit.

`readSpec` and `mountIslands` are exported too, for a page that mounts islands without `BasePage`. `mountIslands` takes a `defer` option for load strategies, and reports what the overlay shows through its `onMount` and `onSkip` options.
