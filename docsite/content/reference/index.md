---
title: "Reference"
description: "The packages, their API docs, and what each version provides."
---

goapplib, `@panyam/tsappkit` and `@panyam/tsappkit-solid` share one version, so `v0.6.8` in `go.mod`, for example, is the same release as `0.6.8` in `package.json`. [CAPABILITIES.md](https://github.com/panyam/goapplib/blob/main/CAPABILITIES.md) says what each version provides.

| Package | Install | API docs |
|---|---|---|
| goapplib (Go) | `go get github.com/panyam/goapplib` | [pkg.go.dev](https://pkg.go.dev/github.com/panyam/goapplib) |
| `@panyam/tsappkit` | `pnpm add @panyam/tsappkit` | [npm](https://www.npmjs.com/package/@panyam/tsappkit) |
| `@panyam/tsappkit-solid` | `pnpm add @panyam/tsappkit-solid` | [npm](https://www.npmjs.com/package/@panyam/tsappkit-solid) |

The Go packages' API, generated from the source on `main` every time the site builds, is on [Go API](api/), and the user-profile service on [UsersService](users/). The TypeScript packages' API is in their sources, which ship in each npm package as `src/`, and the [guide]({{.Site.PathPrefix}}/guide/) covers what they do.
