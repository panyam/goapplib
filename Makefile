# goapplib Makefile
#
# Usage:
#   make setup    - Configure git hooks and local dev environment
#   make test     - Run all tests
#   make help     - Show available targets

.PHONY: setup test wasm-test exercise-wasmhost exercise-wasmhost-gen exercise-islands exercise-worker-state help

GOROOT_WASM := $(shell go env GOROOT)/lib/wasm
EXERCISE := exercise/wasmhost
ISLANDS := exercise/islands
WORKERSTATE := exercise/workerstate

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2}'

setup: ## Configure git hooks and local dev environment
	git config core.hooksPath .githooks
	@echo "Pre-push hook activated (.githooks/pre-push)"

test: ## Run all tests
	go test ./...
	$(MAKE) wasm-test
	cd tsappkit && pnpm install --frozen-lockfile && pnpm test
	cd tsappkit-solid && pnpm install --frozen-lockfile && pnpm test

# The timeout matters: a wasmhost export that blocks the event loop hangs rather than failing.
wasm-test: ## Run the wasmhost tests as wasm under Node
	PATH="$(GOROOT_WASM):$$PATH" GOOS=js GOARCH=wasm go test -timeout 60s ./wasmhost/...

exercise-wasmhost: ## Mission #33 exercise: a toy Connect service as wasm in a Web Worker, in headless Chromium
	cd $(EXERCISE) && pnpm install --frozen-lockfile
	mkdir -p $(EXERCISE)/dist
	GOOS=js GOARCH=wasm go build -buildvcs=false -o $(EXERCISE)/dist/files.wasm ./$(EXERCISE)/wasm
	cp "$(GOROOT_WASM)/wasm_exec.js" $(EXERCISE)/web/index.html $(EXERCISE)/dist/
	cd $(EXERCISE) && pnpm exec esbuild web/main.ts --bundle --format=esm --outfile=dist/main.js --log-level=warning
	cd $(EXERCISE) && pnpm exec esbuild ../../tsappkit/src/wasmhost/worker.ts --bundle --format=iife --outfile=dist/worker.js --log-level=warning
	cd $(EXERCISE) && node run.mjs

exercise-wasmhost-gen: ## Regenerate the exercise's Go and TS code from its proto
	cd $(EXERCISE) && pnpm install --frozen-lockfile
	cd $(EXERCISE)/proto && buf generate

exercise-islands: ## Mission #60 exercise: a page whose islands load and mount by strategy, in headless Chromium
	cd $(ISLANDS) && pnpm install --frozen-lockfile
	mkdir -p $(ISLANDS)/dist
	go build -buildvcs=false -o $(ISLANDS)/dist/server ./$(ISLANDS)
	cd $(ISLANDS) && node build.mjs
	cd $(ISLANDS) && node run.mjs

exercise-worker-state: ## Mission #74 exercise: a stateful service in a Web Worker, its memory, a reload, long jobs and an abort, in headless Chromium
	cd $(WORKERSTATE) && pnpm install --frozen-lockfile
	mkdir -p $(WORKERSTATE)/dist
	GOOS=js GOARCH=wasm go build -buildvcs=false -o $(WORKERSTATE)/dist/state.wasm ./$(WORKERSTATE)/wasm
	cp "$(GOROOT_WASM)/wasm_exec.js" $(WORKERSTATE)/web/index.html $(WORKERSTATE)/dist/
	cd $(WORKERSTATE) && pnpm exec esbuild web/main.ts --bundle --format=esm --outfile=dist/main.js --log-level=warning
	cd $(WORKERSTATE) && pnpm exec esbuild ../../tsappkit/src/wasmhost/worker.ts --bundle --format=iife --outfile=dist/worker.js --log-level=warning
	cd $(WORKERSTATE) && node run.mjs
