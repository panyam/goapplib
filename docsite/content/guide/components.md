---
title: "Components and the page lifecycle"
description: "How tsappkit starts a page's components in phases, what runs when, and how a late island's start differs from the page's."
---

A page has a store that loads data, a counter that shows it, and a button inside the counter that changes it. The counter needs the store before it can subscribe. The button needs the store too, but only the counter knows about the button. If each component set itself up in its constructor, the order of the `new` calls would decide whether the counter finds a store. tsappkit's `LifecycleController` takes that order away. It starts components in phases, and each phase finishes for the whole tree before the next one begins.

The demo below is that page. The log lists every lifecycle call in the order it ran. After the page has started, the demo mounts one more component on a controller of its own, the way a deferred [island](../islands/) mounts. The +1 button works, and both the counter and the late badge follow it.

{{ demo "lifecycle" 360 }}

## The phases

A component that takes part implements `LCMComponent`, short for lifecycle-managed component. It has four methods:

1. `performLocalInit()` sets the component itself up and returns its children. The controller walks the tree breadth-first, so it calls the root, then every child the root returned, then their children. Within one level, one component's call finishes before the next one's starts, and a returned Promise is awaited.
2. `setupDependencies()` runs on every component, in the same order, after every `performLocalInit` has finished. All components exist by now, so a parent hands its children what they need: a sibling, a service, or itself. The controller doesn't await a Promise returned here, so async work belongs in `activate`.
3. `activate()` starts the component working: subscribing to events, loading data, rendering. Each level's calls run together, and the next level starts once all of them have resolved.
4. `deactivate()` runs when `deactivateAll()` tears the tree down. The leaves go first, so a parent deactivates after its children.

In the log, `store activate` starts before `counter activate` and finishes after it. The counter subscribed to `count` while the store was still loading, so it caught the store's first event (`counter got count 3`). The button is a level further down, so its `activate` waits for both.

A constructor only stores its arguments and creates child objects. Work there runs before any phase, in whatever order the `new` calls happen to come in.

A phase that throws or rejects rejects `initializeFromRoot`, and the phases after it don't run, so nothing activates. A component returned by two parents is an error, since the tree has to be a tree. A controller starts one tree once, and a second `initializeFromRoot` on it throws.

## BaseComponent and the EventBus

`BaseComponent` implements `LCMComponent` with empty phases, so a component overrides only the phases it uses. It also gives a component:

- a `rootElement`, marked with `data-component="<id>"` for debugging, plus `findElement` and `findElements` scoped to it;
- an `eventBus`, the one passed to its constructor, or a new one;
- `emit(type, data, subject)`, and `addSubscription(type)`, which routes events to the component's `handleBusEvent(type, data, subject, emitter)`.

The `EventBus` is synchronous. `emit` calls every subscriber before it returns, in the order they subscribed. A subscriber that throws is logged with `console.error`, and the rest still get the event. `on`, `off` and `once` take plain functions, for code that isn't a component. Components on one page share one bus, which the page creates and passes down.

## BasePage

`BasePage` is the root component of a page. Its root element is `document.body`. Its constructor sets up the pieces every page has: the theme (`ThemeManager`), `Modal`, `ToastManager`, and the theme toggle button if the layout has one. A page subclass fills in two hooks:

- `initializeSpecificComponents()` runs in `performLocalInit` and returns the page's top-level components.
- `bindSpecificEvents()` runs in `activate`, after every `setupDependencies` and before any child activates.

`setupDependencies` isn't a hook. A page that wires its children overrides it, the way the demo's page hands the counter its store. `BasePage.loadAfterPageLoaded(name, PageClass, className)` starts the page on `DOMContentLoaded`. It constructs the page with `className` as its id, puts it on `window[name]` so a test can reach it, and runs a `LifecycleController` from it.

Here is the demo's code. `trace` writes a line to the log.

```ts
{{ includeFileText "demos/lifecycle/components.ts" }}
```

The demo starts the page the same way `loadAfterPageLoaded` does:

```ts
const page = new DemoPage(trace);
await new LifecycleController(page.eventBus).initializeFromRoot(page);
```

## Islands and late components

`IslandPage` extends `BasePage`. An eager island's component is one of the page's children, returned from `initializeSpecificComponents`, so it goes through the page's phases with everything else.

An island that mounts later (`idle`, `visible`, `media:`, or any `lazy` registry entry) starts after the page's controller has finished, so `IslandPage` gives it a `LifecycleController` of its own. It still gets all three phases, but its tree is just itself. Nothing calls its `setupDependencies` with the page's components, and nothing orders its `activate` relative to the page's. In the demo, the badge mounts this way, which has two effects:

- It gets the store in its constructor, the way an island's factory gets the page's context, rather than from a parent in `setupDependencies`.
- The store's first `count` event went out before the badge subscribed, so the badge reads `store.count` in `activate` and then follows the events.

The islands guide covers [when each island mounts](../islands/#when-each-island-mounts), and why a component the page needs at startup shouldn't be deferred.
