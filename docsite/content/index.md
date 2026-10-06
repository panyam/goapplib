---
title: "goapplib"
description: "Server-rendered Go web apps, with islands of client code and Go handlers that can run in the browser."
hideTitle: true
---

<div class="home-hero">
<h1>goapplib</h1>
<p class="hero-subtitle">A Go library for server-rendered web apps. Pages are Go views rendered through templates, the parts that need client code are islands mounted by a small TypeScript kit, and an app's own Go handlers can run in the browser as wasm when a round trip to the server is too slow.</p>
<div class="hero-actions">
<a href="{{.Site.PathPrefix}}/overview/" class="btn btn-primary">What goapplib is</a>
<a href="{{.Site.PathPrefix}}/guide/getting-started/" class="btn btn-secondary">Getting started</a>
<a href="https://github.com/panyam/goapplib" class="btn btn-outline">GitHub</a>
</div>
</div>

<div class="features">
<div class="feature-card">
<h3>Pages, mixins and htmx</h3>
<p>Views load their own data, mixins add pagination, auth and filtering, and the same templates render whole pages and htmx fragments.</p>
<a href="{{.Site.PathPrefix}}/overview/#goapplib">The Go library &rarr;</a>
</div>
<div class="feature-card">
<h3>Islands</h3>
<p>A page names its islands and their slots in Go; tsappkit mounts each one when its load strategy says to: at once, when the browser is idle, when its slot scrolls into view, or when a media query matches.</p>
<a href="{{.Site.PathPrefix}}/overview/#tsappkit">The TypeScript kit &rarr;</a>
</div>
<div class="feature-card">
<h3>Go handlers in the browser</h3>
<p>wasmhost runs an app's HTTP or Connect handlers as wasm in a Web Worker, over files the page hands it, and the page's clients can't tell it from a server.</p>
<a href="{{.Site.PathPrefix}}/overview/#wasmhost">The worker host &rarr;</a>
</div>
</div>
