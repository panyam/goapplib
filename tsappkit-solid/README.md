# @panyam/tsappkit-solid

Solid islands for [@panyam/tsappkit](https://www.npmjs.com/package/@panyam/tsappkit). Solid's reactivity stays in this package, so tsappkit and the presenters behind your islands don't depend on a UI framework.

```bash
npm install @panyam/tsappkit-solid @panyam/tsappkit solid-js
```

It needs `@panyam/tsappkit` (any 0.x) and `solid-js` 1.9 or a later 1.x. It shares one version with tsappkit and goapplib, so `@panyam/tsappkit-solid@0.7.7` goes with `@panyam/tsappkit@0.7.7` and goapplib `v0.7.7`.

## What's in it

- **`SolidIsland`** is a tsappkit component that renders a Solid tree into an island's slot. It renders in `activate`, so the tree can use whatever was injected in `setupDependencies`, and disposes of it in `deactivate`. It replaces the placeholder Go rendered in the slot in the same task, so the slot never shows empty.
- **`signalView`** makes a signal whose setter is a view method a presenter can call, such as `setPicks`.
- **`useView`** attaches a view to a presenter's `Port` while the island is active and detaches it when the island goes.

An island that shows a presenter's history, mounted from an `IslandPage` registry:

```tsx
import { SolidIsland, signalView, useView } from "@panyam/tsappkit-solid";

export default function history(el: HTMLElement, island: IslandSpec, ctx: { editor: EditorPresenter }, bus: EventBus) {
  return new SolidIsland("history", el, () => {
    const [picks, setPicks] = signalView<readonly Color[]>([]);
    useView(ctx.editor.history, { setPicks });
    return <p>History: {picks().join(", ")}</p>;
  }, bus);
}
```

See [One presenter, several islands](https://panyam.github.io/goapplib/guide/islands/#one-presenter-several-islands) for the presenter side, and [Islands](https://panyam.github.io/goapplib/guide/islands/) for island pages.

## License

Apache-2.0
