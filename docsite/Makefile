.PHONY: run build demos node-deps test check exercise clean

export GOFLAGS := -buildvcs=false

# Serve the site at http://localhost:8085/goapplib/. It builds once and has no file watcher, so
# restart it after an edit.
run: demos
	GOAPPLIB_DOCS_PORT=:8085 go run .

node-deps:
	pnpm install --frozen-lockfile

# Bundle the live demos (demos/<name>/) into static/demos/, which git ignores.
demos: node-deps
	node build.mjs

# Build the site into ./dist. s3gen writes pages only, so static/ is copied in after; without it the
# built site has no CSS, scripts or demos. .nojekyll stops GitHub Pages from dropping files that
# start with an underscore.
build: demos
	go run . -build
	rm -rf dist/static
	cp -r static dist/static
	touch dist/.nojekyll

# The docsite's tests: templates parse, navigation is wired, and every internal link in the built
# site resolves (which needs the build first).
test: build
	go test -count=1 ./...

# Mission #91's exercise: build and test the site, then load every page and run every live demo in
# headless Chromium (run.mjs). Screenshots land in dist-screenshots/.
exercise: test
	node run.mjs

check: exercise

clean:
	rm -rf dist dist-screenshots static/demos
