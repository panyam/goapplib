# The Presenter-Contract Architecture (goapplib design reference)

## What this document is

This is the design reference for building apps on goapplib and tsappkit where the
business logic lives in one language of your choice, runs unchanged on the server and in
the browser, and drives a frontend that can be written in any UI stack (vanilla, Solid,
Vue, htmx, or none at all) without the logic knowing which.

It is app-agnostic. It is meant to be read by any project that consumes goapplib
(`@panyam/tsappkit`, `@panyam/tsappkit-solid`, the Go libraries), not by one app in
particular. The reference implementation named throughout is those shared packages; a
concrete end-to-end instantiation is in the appendix as one project's case study, but the
principles do not depend on it.

The claims here are opinionated. Where there is a genuine fork, this document names it as
a fork rather than pretending there is one right answer.

## The core idea in one paragraph

Put all business logic behind a service interface. Run that same logic in three places:
the server (over the network), the browser (over WASM), and tests (in process). Between
the logic and the screen, define a typed, two-way contract expressed as **typed view
interfaces**: the view sends the logic semantic intents (what the user meant), and the
logic sends the view commands (what should now be true on screen) by calling methods on
an interface it declares. The frontend is a renderer of that contract and an emitter of
intents. Because the frontend only ever speaks the contract, and because the contract is
typed method calls rather than a message bus or a shared store, the UI stack behind it is
an implementation detail you can change or replace without touching the logic, and the
compiler verifies the wiring.

That is the whole thesis. The rest is detail, tradeoffs, and the parts that are easy to
get subtly wrong.

## The three independent axes

A common misreading of this architecture is "it is a WASM architecture." It is not. WASM
is one option on one of three axes that vary independently. Keeping them separate is what
lets a project adopt the whole thesis with no WASM at all, or with WASM on one surface and
none on another.

1. **Does this surface need a presenter at all?** A presenter earns its place only where
   there is an interactive intent loop worth abstracting. A static content page or a
   read-only list has no such loop. There, the server renders and there is no presenter.
   The presenter is optional, per surface.

2. **If it needs a presenter, what language and runtime hosts it?** Server (over the
   network), browser via WASM, or in-process in the frontend language itself. This is
   chosen per surface by latency budget, not globally. A surface whose interactive loop
   fires on every click can afford a WASM or server round-trip. A surface whose loop fires
   on every `pointermove` or every frame cannot, so its presenter lives in-process next to
   the render loop.

3. **How formal is the contract?** Match formality to transport. A presenter that crosses
   a process boundary (WASM or server) needs a serializable, generated interface (proto is
   a good fit) because the boundary is real and must be enforced. An in-process presenter
   in the frontend language can use a plain hand-written interface with no serialization
   tax. Same duplex shape, different amount of ceremony, chosen by whether a wire is
   actually crossed.

The payoff of separating these: the "swap the frontend" property comes from the contract,
not from any runtime. A project could put every presenter in plain frontend-language code,
ship zero WASM, and still get the full benefit. WASM is an optimization you reach for on
the specific surfaces whose latency budget or offline requirement justifies shipping the
logic tier to the client. It is never a precondition.

## Principle 1: One logic tier, three runtimes

The business logic is a set of services defined by an interface (in practice, proto
service definitions, but the idea does not require proto). The same implementation runs:

- On the **server**, reached over the network (gRPC / Connect / REST).
- In the **browser**, compiled to WASM and called in-process by the page.
- In **tests and CLI drivers**, called directly.

There is one implementation of the logic, not three. The runtimes differ only in transport
and in where state is persisted. This is what makes "local-first with server fallback" a
transport choice rather than a second codebase. An action applied locally in WASM and the
same action applied on the server run the exact same code path.

The discipline this requires: the logic must not assume it is on a server (no direct DB
handles baked into the core) and must not assume it is in a browser (no `syscall/js` in the
core). Persistence and platform effects are injected at the edge. The core is pure with
respect to where it runs.

## Principle 2: The view contract is a duplex, and both directions are semantic

Most "the backend drives the UI" designs describe only the downward direction (logic tells
the view what to show). That is half the loop. The other half is how user actions get back
to the logic. Name both, and hold both to the same standard.

- **Up (view to logic): intents.** `unitSelected(pos)`, `optionClicked(index, type)`,
  `endTurnClicked()`. An intent says what the user meant in the vocabulary of the domain.
  It does not carry DOM nodes, pixel offsets, or event objects. The logic is the update
  function that receives intents.
- **Down (logic to view): commands.** `showHighlights(cells)`, `setStats(data)`,
  `movePiece(from, to)`. A command says what should now be true on screen, in domain terms
  where possible.

This is the Model-View-Intent / Elm-architecture loop, split across a process boundary.
Intent up, effects down, logic in the middle.

The single most important rule in this whole document:

> Intents and commands must never carry framework or DOM specifics. `optionClicked(index,
> type)` is portable across every UI stack. `onButtonClick(domEvent)` is not, and
> `setState(solidSignal)` is not either. If a contract method takes an event object, a DOM
> node, or a framework type, the presenter is coupled to one view and the decoupling is a
> fiction.

This rule has a home, and naming it clarifies what the view layer is for. The view has a
job in **both** directions, and they are symmetric:

- Inbound: translate raw platform input (pixels, DOM events, key codes) into semantic
  intents. A pointer handler computes the cell under the cursor and calls `cellClicked(q,
  r)`. The presenter never sees a pixel or an event object.
- Outbound: translate semantic commands back into platform output (DOM, canvas draws,
  audio).

So the renderer is an adapter on both edges: raw-input-to-intent up, command-to-output
down. This is precisely why the frontend stack is swappable. Everything platform-specific
lives in that adapter, and the presenter is written against neither the DOM nor a
framework. It also means the rule survives the hardest case: a drag loop firing on every
`pointermove` can still keep intents semantic, because the pixel-to-domain conversion
happens in the adapter before the intent is emitted, not inside the presenter.

## Principle 3: The contract mechanism is typed view interfaces, not a bus and not a shared store

Principle 2 says what flows. This principle says *how*. The mechanism is **typed method
calls on interfaces**, and the choice of mechanism matters as much as the choice of
altitude.

- The presenter depends only on a **typed view interface it declares** (its "port"). It
  imports no framework. Commands down are typed method calls on that interface.
- The view/island **implements** the interface. Intents up are typed calls on the
  presenter.
- The framework is parameterized in at the composition root, not baked into the presenter.
  Think `App<Presenter>` / dependency inversion: the abstraction (the interface) sits
  between presenter and view, and the framework-specific concretion depends on it, never
  the reverse.

```ts
// Framework-neutral, declared with the presenter. No framework import.
interface ToolsView { setToolState(s: ToolState): void; }

class EditorPresenter {
  constructor(private tools: ToolsView) {}
  selectTerrain(t: number) { this.tools.setToolState({ ...this.state, terrain: t }); }
}
```

The exact same presenter, unchanged, wires to any framework at the composition root; only
the island that implements `ToolsView` differs. The compiler checks that the island
implements the interface and that it calls the presenter's real intent methods.

**Why not an EventBus.** A global, string-keyed pub/sub is untyped (a typo silently
no-ops), untraceable (broadcast has no static "who emitted this"), and cycle-prone (a
handler emits an event that re-triggers its own emitter). Projects that started on an
EventBus for the presenter/view path moved off it precisely because message flow became
impossible to reason about and formed cycles. An EventBus is still fine for *intra-view*
coordination between sibling widgets; it is not the presenter contract.

**Why not a shared observable store either.** A store the presenter writes and the view
subscribes to is better than a bus (single typed cell, read-only observers, no write-back,
so no cycle path). But it is still indirection in the contract, and it still puts a
`subscribe` where a typed method call belongs. Typed method calls are stronger: the write
site is the trace, and the compiler validates the wiring. Reserve reactive primitives for
*inside* the view (see below), not for the contract.

**Where reactivity lives.** The framework's reactive primitive (a Solid signal, a Vue ref,
a Preact signal) lives **inside the island**, invisible to the presenter. The island
implements a command-down method by writing that primitive. So "how reactivity happens" is
framework-specific and confined to the leaf; the presenter only ever makes typed calls.

**The presenter is framework-agnostic two ways.** In the frontend language (TS) it is
agnostic **by discipline**: keep framework types out of the interface, enforce with review
or a grep. Across a WASM or network boundary it is agnostic **by construction**: only
serializable data crosses, so a Go-in-WASM presenter physically cannot leak a Solid signal
or a JSX node into a view call even if it tried. The serialization boundary is the
discipline made unavoidable. A quick review check: `grep -r "solid-js|from 'react'|from
'vue'"` over the presenter/core should return nothing.

## Principle 4: Altitude decides how portable the payload is

Given the mechanism is typed method calls (Principle 3), the payload those calls carry
comes in two altitudes, and the choice has consequences.

- **Semantic (high altitude):** `setStats(data)`. The method carries domain data. The view
  decides how to render it. Any framework can implement it. You can swap renderers freely.
- **Presentational (low altitude):** `setStatsContent(htmlString)`. The method carries
  rendered markup. The logic tier has already decided the presentation. The view is an
  injection sink.

Presentational commands are not wrong. They are exactly how htmx, Phoenix LiveView, and
Hotwire work: the logic tier renders HTML and the client is a dumb sink. That is a
coherent, modern school, not a legacy compromise. But be clear-eyed about the cost: with
presentational commands, the frontend framework question is moot for that surface, because
there is no view logic to put in a framework. To later "render this panel in Solid" you
must first pull the rendering back out of the logic tier. You cannot have both "logic
renders the HTML" and "framework owns the rendering" for the same surface.

Both altitudes still travel over the same typed method call: `setStatsContent(html)` and
`setStats(data)` are both commands on the view interface. The mechanism (Principle 3) is
constant; only the payload altitude changes.

A useful default: keep the **canvas / bespoke-interactive** surfaces semantic (a canvas
cannot take an HTML string anyway, so its commands are naturally domain-level like
`showHighlights` and `movePiece`), and make a **deliberate** choice for the
**document/panel** surfaces between the two schools in "The fork" below.

## Principle 5: Make the semantic command the primitive

Because the frontend only speaks the contract, the renderer is swappable. The strongest
version of this exposes the **semantic** command as the primitive and treats each rendering
strategy as an implementation on top of it. That gives an application three interchangeable
choices behind one interface:

1. **Server-rendered HTML.** The server produces markup from the semantic data. Good for
   content-heavy or SEO-sensitive apps that do not need local compute. No WASM on the
   critical path.
2. **Logic-rendered HTML in the browser (the LiveView-in-WASM path).** The logic tier,
   running in WASM, renders HTML and ships it down. Good for rich local interactivity with
   the logic tier as the single source of both state and presentation.
3. **Bring-your-own-framework.** A Solid/Vue/Preact island consumes the semantic commands
   and emits intents. Good when the view has its own rich local state and you want
   fine-grained reactivity.

If you build only the presentational command and never the semantic one underneath, you
have quietly locked every application into choice 2 forever. If you build the semantic
command as the primitive and offer the HTML renderer as a default on top, you keep all
three doors open at near-zero cost, because the default still ships.

## Principle 6: Local-first is transport selection plus a reconciliation obligation

Because the same logic runs in WASM and on the server (Principle 1), "local-first" means:
apply the user's intent locally and optimistically in WASM, render immediately, and persist
to the server in the background. The server is the authority; the local copy is an
optimistic prediction.

The easy half is the optimistic apply and the background persist. The hard half, and the
one that is a genuine distributed-systems problem rather than a frontend problem, is
**reconciliation**: what happens when the server rejects an action, or another client's
action arrives, or the two diverge. The obligations are:

- An authoritative downstream channel (a stream or poll) that delivers server-confirmed
  changes, which the logic tier applies over the optimistic local state.
- A defined resolution when local and authoritative disagree (replay authoritative over
  optimistic, roll back the rejected local delta, and surface the correction to the user).
- A visible, non-silent path when a local action is rejected. Diverging state recoverable
  only by a full page refresh is a stopgap, not a model.

If local-first is meant to be a pillar of the project rather than an optimization, the
reconciliation model must be written down and tested, not left implicit. Treat it as a
first-class design artifact.

## Principle 7: Routing belongs to the backend

Navigation is server-owned. Each page is a distinct server-rendered document that boots its
own logic island. There is no client-side router owning the URL.

This is not the old way, it is the current swing away from single-page-app routing (Astro,
htmx, server components, "islands"). It composes naturally with everything above: the server
picks which page and which layout variant to serve (by device, by user, by preference), the
page ships the hidden initial data, and the island boots the logic tier and starts speaking
the contract. Do not add a client router to feel modern. Server routing plus per-page
islands is the more defensible position.

## The fork you must actually decide

For the **document/panel** surfaces (not the canvas, which stays semantic), there are two
coherent futures. They are mutually exclusive for a given surface. Pick per project, and
ideally per surface, on purpose. Both use the typed-view-interface mechanism (Principle 3);
they differ only in the payload altitude (Principle 4).

### Future A: HTML over the wire (LiveView / htmx school)

The logic tier owns rendering and its command methods carry HTML strings. The frontend is a
dumb sink with delegated event handlers that translate clicks into semantic intents.

- Strengths: one place authors presentation (the logic language), no view-state
  duplication, no framework churn, genuinely simple client. Framework "backlash" does not
  apply because you have opted out of client frameworks the way LiveView users have.
- Costs: every panel update ships a string and re-hydrates event handlers after injection.
  No fine-grained reactivity (you re-render whole fragments). Rich client-only interactions
  must escape back to hand-written code. The author writes UI in the logic language plus a
  templating layer, a niche skill set for contributors. In the WASM variant, you ship the
  logic tier to the browser before first paint.

### Future B: Semantic payload plus a framework island

The command methods carry semantic data. A reactive island (Solid/Preact/Vue) implements
the view interface, renders from that data, and binds events declaratively.

- Strengths: fine-grained reactivity, declarative event binding (no re-hydration dance),
  familiar to most contributors, the renderer is swappable. This is the literal realization
  of "frontend in the stack of your choice."
- Costs: presentation logic now lives in the frontend, so the "views as dumb as possible"
  goal is abandoned for that surface. You maintain view logic per frontend if you support
  more than one. More moving parts.

The honest recommendation for a reusable stack: **build the semantic command as the
primitive (Principle 5), ship the HTML-over-the-wire renderer as the default (Future A stays
the path of least resistance), and leave Future B available as an island for the surfaces
that need rich local interactivity.** For an application whose surfaces are mostly forms,
lists, and content rather than a bespoke interactive canvas, prefer server-rendered HTML
(choice 1) as the default and make WASM an opt-in enhancement, so you are not shipping a
megabyte of logic to render a settings page.

## Reference implementation in goapplib

The shared packages are the reference implementation of this thesis. Use them rather than
re-deriving the pieces per app.

**`@panyam/tsappkit` (framework-neutral core).**
- `BaseComponent` implements a breadth-first component lifecycle
  (`performLocalInit` → `setupDependencies` → `activate` → `deactivate`) and owns a
  `rootElement`. This is the mount point and lifecycle a view adapter hooks into.
- `LifecycleController` orchestrates that lifecycle across a page's components with
  synchronization barriers, so components can be declared in any order.
- `EventBus` exists for **intra-view** coordination between sibling widgets. Per Principle
  3, it is not the presenter contract. Do not route presenter commands or intents through
  it.
- The core has zero framework dependencies. That invariant is what keeps every consumer
  free to pick a frontend stack.

**`@panyam/tsappkit-solid` (the Solid adapter, a leaf package).**
- `SolidIsland` extends `BaseComponent` and mounts a Solid root in `activate` (so the
  render function can use dependencies injected during `setupDependencies`) and disposes it
  in `deactivate` (runs Solid `onCleanup`, removes nodes). It is the only place Solid
  reactivity touches the lifecycle.
- `signalView(initial)` backs a typed view-interface method with a Solid signal: the
  accessor drives JSX, the returned setter is the command-down method the presenter calls.
  It always writes via the updater form (`set(() => next)`) so it stays correct even when
  the state type is itself a function, a Solid footgun a shared helper should own once.
- `solid-js` is a **peerDependency**, so the framework is pulled only by apps that opt into
  this adapter.

Putting the ToolsView example together with the adapter:

```ts
// Solid island implements the framework-neutral ToolsView by writing a signal.
const [toolState, setToolState] = signalView<ToolState>(initial);
const view: ToolsView = { setToolState };                 // command down -> signal
const island = new SolidIsland('tools', rootEl,
  () => <ToolsPanel state={toolState()}
                    onSelectTerrain={t => presenter.selectTerrain(t)} />); // intent up
const presenter = new EditorPresenter(view);              // compile-time checked wiring
```

**The dependency-graph discipline.** Framework dependencies live only in leaf adapter
packages (`tsappkit-solid`, a future `tsappkit-vue`), never in `tsappkit` core. The core
exposes framework-neutral contracts and lifecycle; each adapter binds them to one
framework and declares that framework as a peer dependency. The dependency graph mirrors
the architecture: framework code sits only at the swappable leaves, never in the core the
leaves plug into. This is the same principle as the presenter contract, one level up in the
build system.

## The costs, stated plainly

- **WASM before first paint.** Shipping the logic tier to the browser is heavy (payload,
  cold start, GC pauses for some toolchains). Justified for a bespoke interactive app, hard
  to justify for a CRUD screen. This is the main argument for making WASM opt-in per surface
  rather than the universal default.
- **Contributor familiarity.** A logic-in-WASM, HTML-from-the-logic-tier stack is unusual.
  The real friction is not "this is bad," it is "few people I hire have done this." Weigh
  that for anything meant to be widely reused.
- **Reconciliation is the actual risk.** The frontend framework question is low-stakes next
  to getting optimistic local state to converge with an authoritative server. Spend design
  effort there.
- **Altitude drift.** If some commands are semantic and others are presentational and nobody
  is watching, the presentational ones multiply and the swap-the-frontend property quietly
  dies. Keep the semantic primitive real, even when the default renderer is HTML.

## Adopting this in a new project

A checklist for a fresh repo that wants this shape:

1. **Define logic as a service interface.** Proto services are a good fit because they
   generate both server and client stubs, but the pattern only needs a typed interface.
2. **Keep the core runtime-agnostic.** No DB handles and no platform calls in the core.
   Inject persistence and platform effects at the edge so the same core runs on server,
   WASM, and test.
3. **Design the duplex contract as typed view interfaces.** Intents up, commands down, both
   as typed method calls. Not an EventBus, not a shared store. Write the intents first; they
   force you to name what users actually do.
4. **Keep intents and commands framework-free and DOM-free.** This is the rule that makes
   the frontend swappable. Guard it in review; across a WASM/network boundary the wire
   enforces it for you.
5. **Confine reactivity to islands.** Use `SolidIsland` + `signalView` (or the equivalent
   adapter for your framework). The presenter never imports a framework.
6. **Make the semantic command the primitive.** Offer HTML-over-the-wire as one renderer on
   top, not as the only shape of the contract.
7. **Keep framework deps in leaf adapter packages**, never in shared core.
8. **Keep routing on the server.** One document per page, each booting its logic island.
9. **Decide local-first per project.** If yes, write the reconciliation model down before
   you rely on it. If no, the server runtime alone is a complete and simpler product.
10. **Choose the panel future deliberately** (A, B, or semantic-primitive-with-both) and
    record the choice. Do not let it drift.

If you do 1 through 8, the frontend framework becomes a decision you can defer and reverse.
That deferral is the entire payoff.

## Appendix: one project's case study (lilbattle)

lilbattle is one instantiation. It is useful because it runs the same duplex presenter
pattern twice, on two surfaces, with every runtime variable flipped. The paths below are
illustrative and may drift; treat them as an example, not a spec.

**Game viewer (Go presenter in WASM).** The game services run on the server via Connect, in
the browser via WASM, and in tests, from one implementation. The duplex contract is two
proto services: intents up (`GameViewPresenter`: `SceneClicked`, `TurnOptionClicked`, ...)
and commands down (`GameViewerPage`, marked `browser_provided`, implemented in TS and
called from WASM: `SetGameState`, `SetTurnOptionsContent`, `ShowHighlights`, `MoveUnit`,
...). Canvas commands are semantic; panel commands are presentational (templar HTML rendered
inside WASM), so the panels are Future A and the canvas is semantic. The WASM presenter is
framework-agnostic by construction. Local-first is real for compute and server for
persistence, with reconciliation still the open half (divergence resolves by refresh today).

**World editor (pure-TS presenter, zero WASM).** The same pattern with every variable
flipped: an in-process hand-written `WorldEditorPresenter`, chosen in-process because the
interactive loop is mouse-drag painting firing on every `pointermove`, where a WASM
round-trip would be too slow (axis 2). Intents are semantic and DOM-free (`handleTileClick(q,
r)`, `selectTerrain(t)`), with pixel-to-hex conversion in the scene adapter. The contract is
a hand-written TS interface, not proto, because nothing crosses a wire (axis 3).

The two surfaces are the thesis in miniature: identical pattern (duplex, typed intents,
view-as-adapter), different runtime, transport, and formality, each chosen per surface. The
tsappkit-solid work (`SolidIsland`, `signalView`) is what lets the editor's panels move from
hand-rolled DOM to a Solid island behind the same typed `ToolsView`, and lets the game
viewer's panels move to Future B once their commands gain a semantic altitude beneath the
HTML one.

## Summary

The frontend framework is the least important decision in this architecture, and that is the
point. Put the logic in one runtime-agnostic tier. Talk to the screen through a duplex
contract of typed view interfaces, semantic intents up and (ideally semantic) commands down,
not through a bus or a shared store. Let the view be an adapter that translates input to
intents and commands to output, with framework reactivity confined to islands. Treat the
presenter, its host runtime, and its contract formality as three independent axes chosen per
surface. Keep framework dependencies at the leaves. Keep routing on the server. Decide the
panel-rendering school deliberately. Treat local-first reconciliation as the real design
work. Do those, and the UI stack becomes a reversible choice you can make late and change
freely.
