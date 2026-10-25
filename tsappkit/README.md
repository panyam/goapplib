# @panyam/tsappkit

The browser half of [goapplib](https://github.com/panyam/goapplib), a Go framework for server-rendered pages. It has a component lifecycle and event bus, island pages that mount client code into a page Go rendered, presenter ports, and wasmhost, which runs an app's Go handlers as wasm in a Web Worker. It doesn't depend on a UI framework; [@panyam/tsappkit-solid](https://www.npmjs.com/package/@panyam/tsappkit-solid) adds Solid islands.

The guides are at https://panyam.github.io/goapplib/. This page lists what's in the package and links to the guide for each part.

```bash
npm install @panyam/tsappkit
```

goapplib and both packages share one version, so goapplib `v0.7.7` goes with `@panyam/tsappkit@0.7.7`.

## `@panyam/tsappkit`

### Island pages

A Go page writes a spec saying which islands go where and when each loads (`eager`, `idle`, `visible` or `media:<query>`). An `IslandPage` reads it and mounts each island from a registry:

```ts
import { IslandPage, lazy } from "@panyam/tsappkit";

class HomePage extends IslandPage<Ctx> {
  protected registry() {
    return {
      player: lazy(() => import("./islands/player")), // default export is the factory
      chat: (el, island, ctx, bus) => new ChatIsland(el, ctx, bus),
    };
  }
  protected makeContext(): Ctx {
    return { api: new ApiClient() };
  }
}

IslandPage.loadAfterPageLoaded("homePage", HomePage, "HomePage");
```

Also here: `readSpec`, `mountIslands`, `scheduleMount` and `IslandOverlay` (add `?islands` to a page's URL to see each slot labelled). See [Islands](https://panyam.github.io/goapplib/guide/islands/).

### Presenter ports

A `Port<V>` lets one presenter drive several views, such as two islands showing the same state. `each` sends a command to every attached view, and the function given to the constructor replays the current state to each view as it attaches, so an island that mounts late catches up:

```ts
import { Port } from "@panyam/tsappkit";

class EditorPresenter {
  private color = "red";
  readonly tools = new Port<ToolsView>((v) => v.setColor(this.color));

  pick(c: string) {
    this.color = c;
    this.tools.each((v) => v.setColor(c));
  }
}
```

See [One presenter, several islands](https://panyam.github.io/goapplib/guide/islands/#one-presenter-several-islands).

### Components and events

- **`LifecycleController`** runs components through their phases breadth-first: `performLocalInit` (which returns child components), `setupDependencies`, then `activate`. Every component is set up before any activates, so startup order stops mattering.
- **`BaseComponent`** is a base class for components that use the `EventBus`.
- **`EventBus`** delivers events to subscribers, and an error in one subscriber doesn't stop the rest.
- **`BasePage`** is a page built from components. `IslandPage` extends it.

```ts
import { BasePage, type LCMComponent } from "@panyam/tsappkit";

class MyPage extends BasePage {
  protected initializeSpecificComponents(): LCMComponent[] {
    return [];
  }
  protected bindSpecificEvents(): void {}
}

BasePage.loadAfterPageLoaded("myPage", MyPage, "MyPage");
```

### UI pieces

`Modal`, `ToastManager`, `ThemeManager` (light, dark or the system's), `SplashScreen`, `MobileBottomDrawer`, `TemplateLoader`, and `KeyboardShortcutManager` for multi-key shortcuts, with `shouldIgnoreShortcut` and the other `DOMUtils` helpers.

## `@panyam/tsappkit/wasmhost`

The page side of wasmhost. The Go side builds an app's own HTTP or Connect handler to wasm with `goapplib/wasmhost`, and the page talks to it through a fetch that never reaches the network:

```ts
import { mountFiles, startWorker, workerFetch } from "@panyam/tsappkit/wasmhost";

const worker = await startWorker({ worker: "worker.js", wasm: "app.wasm", exec: "wasm_exec.js", ns: "files" });
await mountFiles(worker, "docs", { "hello.txt": new TextEncoder().encode("hello") });
// Any client that takes a fetch works, Connect's included. Only the path and query reach the handler.
const res = await workerFetch(worker)(location.origin + "/read?path=docs/hello.txt");
```

The worker script ships prebuilt as `@panyam/tsappkit/wasmhost/worker.js`. Serve it beside your bundle, from the same version as the rest of the package. `addFiles`, `filesFromDrop` and `filesFromFileList` fill the mounts, and `workerMemory` and `workerStats` report the worker's memory and its Go heap. See [Go handlers in a Web Worker](https://panyam.github.io/goapplib/guide/wasmhost/).

For state that survives a reload, a job with a big memory peak (`oneShotFetch`), or long jobs on a worker of their own that can be aborted, or replaced when it grows (`startLane`), see [State, heavy jobs and long requests](https://panyam.github.io/goapplib/guide/wasmhost-state/).

## `@panyam/tsappkit/docs`

Pieces for documentation sites built with s3gen: `initCodeBlocks` and `CodeBlock` add a copy button to code blocks, `initPageSetup` and `highlightActiveSidebarLink` handle the sidebar, and `ActionCompiler` compiles and runs code typed into an Ace editor (`ace-builds` is an optional peer dependency, needed only for this). The SCSS is in `@panyam/tsappkit/styles/docs/`, and the package ships it compiled as `dist/docs/DocsPage.css`. See [Documentation site best practices](https://github.com/panyam/goapplib/blob/main/tsappkit/docs/BESTPRACTICES.md).

## License

Apache-2.0
