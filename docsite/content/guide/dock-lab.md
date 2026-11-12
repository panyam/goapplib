---
title: "Dock experiments"
description: "Two experiments on tsappkit-dock, run live in your browser: drawers pinned beside the grid, and panels popped out into their own windows."
---

[Docks and drawers](../dock/) covers what tsappkit-dock does today. This page is where we try what it might do next. Each experiment below is a live demo with a table of questions, and pressing its Run button answers them in your browser. The numbers in the text come from headless Chromium 153 and WebKit 26.6, which gave the same answers. Firefox and real Safari are worth a run, especially for the popup-blocker rows.

## Pinned drawers (#138)

A reader opens the Checks drawer and works through findings one by one, and each click locates a subject on the canvas. An overlay drawer covers part of the drawing they're trying to see. A pinned drawer would push the grid aside instead, so the drawing keeps its full view.

The demo prototypes pinning from outside tsappkit-dock. The dock's container sits in a stage, the drawers mount on the stage through `drawerHost`, and a pinned drawer hides its scrim and insets the container by its own size. Open a drawer, press Pin, and drag its edge.

{{ demo "dock-pinned" 760 }}

What we found:

- **dockview keeps up.** Dragging a pinned drawer's edge over 60 frames kept every frame at the 16.7 ms budget, and still did with the CPU throttled 6×. The grid's edge met the drawer's edge at every step. The cost goes elsewhere. A pinned drag resized the canvas panel on 59 of the 60 frames, and an overlay drag resized it on none. This canvas is cheap to redraw, but a WebGL canvas reallocates and redraws on each one.
- **For the canvas, a pinned drawer and a docked panel are the same thing.** With Checks pinned on the right, the canvas got 665×385. With Checks docked in the grid at the same width, it got 662×385, the 3 px being dockview's sash. The visible difference is that the docked panel gains a 35 px tab header, and it can be dragged and split.
- **Adjacent pinned drawers need a corner rule, and either rule works.** With left and bottom both pinned, "left takes the corner" and "bottom takes the corner" both left no overlap between the two drawers or the grid.
- **No API change is needed for the host.** Pinning shrinks the dock's container, so a drawer mounted inside the container would shrink with it. `drawerHost` already lets drawers mount outside it.

**Recommendation: no pinned mode. Pin means dock.** A pinned drawer gives the canvas the same box as docking the panel, and it adds a third way of placing a panel that readers would have to learn. The ⋯ menu's Dock already does it. Dock from a drawer should land on that drawer's edge at the drawer's size, though, so the panel stays where the reader was looking. Today it goes to the panel's `position`.

## Popout windows (#139)

A reader with two monitors wants the schematic on one screen and the Checks table on the other. dockview can pop a group out into its own browser window with `api.addPopoutGroup`. The demo pops out a Solid panel, a canvas and a plain-DOM notes box, uses each one there, saves and restores the layout, and docks them back. Your browser may ask to allow pop-ups.

{{ demo "dock-popout" 760 }}

What we found:

- **The element moves into the popout's document, and neither `hostFor` nor `release` is called.** A popout behaves like a float in that respect. The canvas's own pointer listener fires there, and a `ResizeObserver` from the page sees the popout resize.
- **Solid's clicks stop working in a popout.** Solid listens for events once, on the page's document, and a click in the popout bubbles to the popout's document instead. Calling `delegateEvents(["click"], popout.document)` fixes it. Any app shortcut that listens on the page's document misses keys typed in the popout for the same reason.
- **Styles follow, and a theme switch doesn't.** dockview copies the stylesheets into the popout. Switching the dock from dark to light changed the page and left the open popout dark.
- **tsappkit-dock doesn't know about popouts.** `dock.where()` reports a popped-out panel as `grid`. So the Dock action closes it and reopens it at its `position`, or with none in dockview's active group, and both of those can be a popout. Removing the panel and adding it at the grid's edge (`position: { direction: "right" }`) brought every panel home with its content.
- **A reload reopens every popout without a click.** The saved layout holds the popout groups, and `fromJSON` called `window.open` once for each. Headless browsers allow that, even with Chromium's popup blocker switched back on, so we couldn't measure what a blocker does. Run the demo in your browser to see.
- **State survives the round trip, and a canvas doesn't.** The Solid count and the typed notes came through popout, save, restore and dock-back. The canvas drew fine in the popout. Once its window closed and the canvas came home, it could no longer draw, in Chromium and WebKit alike. Its 2D context belonged to the closed window, `contextrestored` never fired, and only a new `<canvas>` element drew again. agni's schematic is a WebGL canvas, so it would need rebuilding after every popout.

**Recommendation: defer popouts and leave them to `dock.api`.** Supporting them would take a `popout` placement, a `where()` that knows the location, `delegateEvents` on each popout document, theme propagation, a restore that doesn't open windows without a click, and a way to tell a panel to rebuild its canvases when it comes home. An app that wants popouts today can call `dock.api.addPopoutGroup` and handle those itself, as this demo does. One fix applies with or without popouts: when `show` or Dock puts a panel in the grid, it should never land in a float or a popout because that group happened to be active.
