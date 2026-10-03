# goapplib

A Go web-app framework (server-rendered pages, mixins, htmx, templates through templar) with the TS packages `tsappkit` and `tsappkit-solid`. See SUMMARY.md for an overview, USAGE_GUIDE.md and INTEGRATION_GUIDE.md for how to use it, and CAPABILITIES.md for the stack-facing API and version. Bump CAPABILITIES.md whenever the API changes.

## Commands

- `make setup` sets up the git hooks. `make test` runs all the tests.
- When a capability lands, tag a release (`v0.x.y`). Consumers (thambura, agni, lilbattle) bump to the tag.

## Issues and missions

Work is queued by the missions it serves. The conventions are in `~/.claude/skills/retriage/CONVENTIONS.md`.

- Active missions: #33 `mission_worker_host` (the Web Worker wasm host, next up #32) and #34 `mission_island_pages` (the page spec and island registry, next up #27).
- A new issue gets a priority (`P0`–`P3`) when it's filed, plus either a `mission_<slug>` label and a blocked-by link from its mission, or `waiting` with its trigger named in the body. Never both P and `waiting`.
- Lifts from apps (thambura, agni) are `waiting` until a second app needs them. The issue body names that app.
- To see what's next, run `~/.claude/skills/retriage/queue.sh`.
