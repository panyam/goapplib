---
title: "Demos"
description: "Every live demo on this site in one place: islands, the component lifecycle, tours, the dock, and Go handlers in a Web Worker."
---

Each demo below runs in your browser. It's built from [`docsite/demos/`](https://github.com/panyam/goapplib/tree/main/docsite/demos) against the goapplib on `main`, and `make exercise-docsite` drives every one in headless Chromium before the site deploys. Each card links the demo on its own, the guide section that explains it, and its source.

## Islands and components

<div class="features">
<div class="feature-card">
<h3>When each island mounts</h3>
<p>Four lazy islands, one per load strategy: at once, when the browser is idle, when the slot scrolls into view, and when a media query matches. A table shows which chunks have downloaded so far.</p>
<p><a href="{{.Site.PathPrefix}}/guide/islands/#when-each-island-mounts">Guide</a> · <a href="{{ assetURL "static/demos/islands-load/index.html" }}">Open</a> · <a href="https://github.com/panyam/goapplib/tree/main/docsite/demos/islands-load">Source</a></p>
</div>
<div class="feature-card">
<h3>A Solid island over a placeholder</h3>
<p>Go renders a placeholder into the slot, and a lazy SolidIsland takes the slot over once its chunk arrives.</p>
<p><a href="{{.Site.PathPrefix}}/guide/islands/#placeholders-and-one-owner-per-region">Guide</a> · <a href="{{ assetURL "static/demos/islands-solid/index.html" }}">Open</a> · <a href="https://github.com/panyam/goapplib/tree/main/docsite/demos/islands-solid">Source</a></p>
</div>
<div class="feature-card">
<h3>One presenter, several islands</h3>
<p>One presenter drives two Solid islands through a Port each. The second island mounts only when you scroll to it, and it still shows every pick you made.</p>
<p><a href="{{.Site.PathPrefix}}/guide/islands/#one-presenter-several-islands">Guide</a> · <a href="{{ assetURL "static/demos/islands-presenter/index.html" }}">Open</a> · <a href="https://github.com/panyam/goapplib/tree/main/docsite/demos/islands-presenter">Source</a></p>
</div>
<div class="feature-card">
<h3>The page lifecycle</h3>
<p>A page starts its components in phases, then mounts a late component. Each lifecycle call goes into a log, in the order it ran.</p>
<p><a href="{{.Site.PathPrefix}}/guide/components/">Guide</a> · <a href="{{ assetURL "static/demos/lifecycle/index.html" }}">Open</a> · <a href="https://github.com/panyam/goapplib/tree/main/docsite/demos/lifecycle">Source</a></p>
</div>
<div class="feature-card">
<h3>A guided tour</h3>
<p>The page dims, and the reader does each step in a small app. A step advances when the app reports the action, and a log shows which event moved it.</p>
<p><a href="{{.Site.PathPrefix}}/guide/tour/">Guide</a> · <a href="{{ assetURL "static/demos/tour/index.html" }}">Open</a> · <a href="https://github.com/panyam/goapplib/tree/main/docsite/demos/tour">Source</a></p>
</div>
</div>

## Docks and drawers

<div class="features">
<div class="feature-card">
<h3>A dock of this site's pages</h3>
<p>Open any page of this site as a panel, then move panels between the grid, a float and four drawers. A Solid counter and a notes box keep their state through every move, and a reload brings the layout back.</p>
<p><a href="{{.Site.PathPrefix}}/guide/dock/">Guide</a> · <a href="{{ assetURL "static/demos/dock/index.html" }}">Open</a> · <a href="https://github.com/panyam/goapplib/tree/main/docsite/demos/dock">Source</a></p>
</div>
<div class="feature-card">
<h3>A phone page</h3>
<p>One canvas fills the grid, and every other panel is a tab in a bottom sheet. The bar's buttons open a panel and close it on a second tap. Drag or flick the sheet to resize it.</p>
<p><a href="{{.Site.PathPrefix}}/guide/dock/#phone-pages">Guide</a> · <a href="{{ assetURL "static/demos/dock-phone/index.html" }}">Open</a> · <a href="https://github.com/panyam/goapplib/tree/main/docsite/demos/dock-phone">Source</a></p>
</div>
<div class="feature-card">
<h3>Pinned drawers</h3>
<p>An experiment: a pinned drawer pushes the grid aside instead of covering it. A results table answers the experiment's questions in your browser.</p>
<p><a href="{{.Site.PathPrefix}}/guide/dock-lab/#pinned-drawers-138">Guide</a> · <a href="{{ assetURL "static/demos/dock-pinned/index.html" }}">Open</a> · <a href="https://github.com/panyam/goapplib/tree/main/docsite/demos/dock-pinned">Source</a></p>
</div>
<div class="feature-card">
<h3>Popout windows</h3>
<p>An experiment: each kind of panel goes out into its own window, gets used there, and comes back. A results table records what survived.</p>
<p><a href="{{.Site.PathPrefix}}/guide/dock-lab/#popout-windows-139">Guide</a> · <a href="{{ assetURL "static/demos/dock-popout/index.html" }}">Open</a> · <a href="https://github.com/panyam/goapplib/tree/main/docsite/demos/dock-popout">Source</a></p>
</div>
</div>

## Go handlers in a Web Worker

<div class="features">
<div class="feature-card">
<h3>A Connect service as wasm</h3>
<p>A Go Connect service runs in a Web Worker. The page mounts files into the worker and calls the service through its generated client, with no server involved.</p>
<p><a href="{{.Site.PathPrefix}}/guide/wasmhost/">Guide</a> · <a href="{{ assetURL "static/demos/wasmhost-files/index.html" }}">Open</a> · <a href="https://github.com/panyam/goapplib/tree/main/docsite/demos/wasmhost-files">Source</a></p>
</div>
<div class="feature-card">
<h3>State, lanes and long jobs</h3>
<p>A service keeps its state in the browser's cache, so a reload restores it. Quick queries and long jobs run in separate workers, and the demo streams a job, aborts one and hits a memory limit.</p>
<p><a href="{{.Site.PathPrefix}}/guide/wasmhost-state/">Guide</a> · <a href="{{ assetURL "static/demos/wasmhost-state/index.html" }}">Open</a> · <a href="https://github.com/panyam/goapplib/tree/main/docsite/demos/wasmhost-state">Source</a></p>
</div>
</div>

## Coming next

The views, routing, templates and htmx guides walk through whole apps in [`docsite/examples/`](https://github.com/panyam/goapplib/tree/main/docsite/examples). GitHub Pages can't run their Go server, so they have no live demo yet. [#147](https://github.com/panyam/goapplib/issues/147) runs them in the browser through wasmhost.
