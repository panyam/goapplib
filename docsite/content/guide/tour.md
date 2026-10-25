---
title: "Guided tours"
description: "A tour where the reader does each step: the page dims, the step's targets stay reachable, and the step advances when the app says the reader did the thing."
---

tsappkit's `Tour` walks a reader through an app by having them do each step. It dims the page and cuts a hole over whatever the step lets them reach. The step advances when the app reports that the thing happened, which is a different signal from a click landing on a selector.

Try it below. Start the tour, then click a finding other than F2 on the third step to see the tour end and leave the app where your click put it. The log under the app shows each app event and what the tour did with it.

{{ demo "tour" 300 }}

## Steps advance on app events

A tour is a list of steps written as data, typed by the app's event map:

```ts
interface ViewerEvents {
  checksRun: void;
  findingSelected: { id: string };
}

const tour = new Tour<ViewerEvents>([
  { id: "run", text: "Run the checks.", target: () => runButton,
    advanceOn: { event: "checksRun" }, doItForMe: () => app.runChecks() },
  { id: "list", text: "These are the findings.", target: () => findingsList },
  { id: "pick", text: "Click F2.", target: () => document.querySelectorAll("#findings li"),
    advanceOn: { event: "findingSelected", where: (d) => d.id === "F2" },
    offPath: { event: "findingSelected" } },
]);
tour.attach(bus);
tour.start();
```

- `target` returns one element, a list of them, a rectangle in viewport coordinates, or a mix. Each becomes a hole. A single cell and every row of a table are the same mechanism with a broader `target`. It's called every frame, so a target that hasn't rendered yet shows up when it does.
- `advanceOn` names an app event and, with `where`, a test of its payload. The tour never looks at the DOM, a mutation or a click target to work out what the reader meant.
- A step with no `advanceOn` is something to look at, and the panel shows Next (Done on the last step).
- `doItForMe` puts a Show me button on the step. It should act through the app the way a reader would (`app.runChecks()`, not `tour.next()`), so the step still advances on the app's event.
- Esc and the panel's close button end any tour. `onStep` and `onEnd(reason, step)` report progress, where `reason` is `"done"`, `"cancelled"` or `"offPath"`.

Because the tour is generic over the event map, a step naming an event the app doesn't declare, or a `where` reading a field its payload doesn't carry, fails to compile instead of producing a step that never advances.

Events reach a tour two ways. `tour.attach(bus)` subscribes to every event the steps name, on tsappkit's `EventBus` or anything with `on` and `off`. An app that keeps its own channels calls `tour.notify(type, data)` instead.

## When the reader wanders off

`offPath` lists events that mean the reader went somewhere else. One that matches `offPath` and not `advanceOn` applies `onOffPath`:

- `"end"`, the default, removes the overlay. The app carries on as it would have anyway, so a reader who clicks F1 where the step wanted F2 has F1 selected and has simply left the tour.
- `"stay"` keeps waiting for `advanceOn`.

Naming the same event as `advanceOn` without its `where` catches every other payload of it, as the `pick` step above does.

## An action with no event isn't tourable

If an app emits no event for an action, no step can advance on it, and that's deliberate. A tour step is an assertion about the app's behaviour, much as a test is. An action the app can't report is one nothing outside the app can observe, whether that's a tour, a browser test or an agent driving the page. Writing a tour shows where those gaps are, and the fix is an app event rather than a cleverer listener. A third-party widget inside the app (an editor, a PDF viewer) is wrapped by the app, which re-emits what its events mean.

## Steps about the UI itself

Some steps teach the UI rather than the app: hover for a tooltip, drag a splitter, scroll a list. Those have no app meaning, and making the app emit `tooltipShown` for a tour would be worse. A step can name a DOM event on its own targets instead:

```ts
{ id: "help", text: "Hover the ? to see how findings are sorted.", target: () => help, advanceOn: { dom: "pointerenter" } }
```

It's spelled differently from an app event, so a reader of the tour can see which kind each step is. It matches when an event of that type passes through one of the step's target elements and infers nothing beyond that.

A tour can use `dom` steps throughout, which is how to tour an app that sends no events (one you can't change, or one that isn't tour-native yet). That works, but it gives up three things:

- **It advances on the interaction, not the outcome.** A click on a disabled button, or on one whose action then fails, still advances the step. An app event would say the action finished and what it produced.
- **An asynchronous step has nothing to wait on.** The step advances when the click lands, before a slow result arrives. Waiting for the result would mean watching the DOM for a sign of it, which is the inference the tour refuses to make.
- **Wandering off is barely visible.** Input outside the holes is blocked anyway, so the only off-path choice is a different element inside a hole, and a broad target can't tell one row from another without a step per row.

## The overlay

The dimmed layer blocks pointer and wheel input outside the holes. Input inside a hole reaches the real element with its hover, focus and drag intact, because the holes are cut with an even-odd `clip-path` and a clipped-away area isn't hit-tested. Keyboard input isn't blocked, so a reader can still Tab to the rest of the page. Holes that overlap (padded table rows always do) are merged first, so they don't dim each other. The holes follow their targets when the layout moves, and the panel sits beside them, clear of every hole, or pinned to the bottom or top of a viewport under 600 px wide.

Theme it with custom properties on `:root` or `.tsappkit-tour`:

| Property | What it sets |
| --- | --- |
| `--tsappkit-tour-dim` | the dimmed layer's background |
| `--tsappkit-tour-accent`, `--tsappkit-tour-accent-fg` | the hole rings, focus outlines and the Next button |
| `--tsappkit-tour-panel-bg`, `--tsappkit-tour-panel-fg` | the panel's colors (a dark default under `prefers-color-scheme: dark`) |
| `--tsappkit-tour-panel-width`, `--tsappkit-tour-panel-radius`, `--tsappkit-tour-font` | the panel's size and type |
| `--tsappkit-tour-z` | the overlay's stacking order |

`labels` in the options renames the panel's buttons. `padding` sets how far a hole extends past its target (4 px by default).

## Testing a tour

The step machine runs without layout, so `notify` in a plain unit test covers advancing, going off path and ending. Whether a click inside a hole reaches its target needs a real browser. `make exercise-tour` drives a fixture app's tour in headless Chromium, clicking through the holes the way a reader would, and an app's own browser test can take its tour the same way, acting only through the holes.
