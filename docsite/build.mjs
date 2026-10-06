// Bundles each live demo in demos/<name>/ (an index.html and a main.ts) into static/demos/<name>/,
// which `make build` copies into the site. demos/_lib/ is the code they share, not a demo.
//
// A demo with a page/ directory is an island page. Its bundle is split, so each lazy island is a
// chunk of its own, and page/ is a Go main (internal/islanddemo) that renders index.html from the
// bundle's metafile, the way an app's server would, instead of index.html being copied.
//
// A demo with a wasm/ directory also gets its Go built for the browser (app.wasm), Go's
// wasm_exec.js from the same toolchain, and tsappkit's wasmhost worker (worker.js), bundled from
// the repo's source the way the exercises do it. A demo reaches all three through `assets` in
// _lib/frame.ts. Every file a demo loads is versioned by a hash of its content, so a browser holding
// an older copy from Pages' ten-minute cache fetches the new one: a stale wasm_exec.js beside a new
// app.wasm fails in ways that are hard to read.
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { copyFileSync, existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { build } from "esbuild";
import { solidPlugin } from "esbuild-plugin-solid";

const here = (p) => fileURLToPath(new URL(p, import.meta.url));
const version = (file) => createHash("sha256").update(readFileSync(file)).digest("hex").slice(0, 12);

// A demo may import code from elsewhere in the repo (the exercises' generated Connect clients,
// tsappkit-solid's source), whose imports would otherwise resolve from that directory's
// node_modules, if it has one, and bundle a second copy. tsappkit-solid's own node_modules holds
// tsappkit from npm and another solid-js, and two copies of Solid don't share effects. So
// @panyam/tsappkit is the repo's source, and the rest resolve from this package.
const oneCopy = {
  name: "one-copy",
  setup(b) {
    b.onResolve({ filter: /^@panyam\/tsappkit$/ }, () => ({ path: here("../tsappkit/src/index.ts") }));
    b.onResolve({ filter: /^(@(bufbuild|connectrpc)\/|solid-js(\/|$))/ }, (args) => {
      if (args.pluginData?.oneCopy) return undefined;
      return b.resolve(args.path, { kind: args.kind, resolveDir: here("."), pluginData: { oneCopy: true } });
    });
  },
};

const goroot = execFileSync("go", ["env", "GOROOT"], { encoding: "utf8" }).trim();
const out = "static/demos";
rmSync(out, { recursive: true, force: true });
for (const name of readdirSync("demos").filter((n) => !n.startsWith("_"))) {
  const dir = `${out}/${name}`;
  mkdirSync(dir, { recursive: true });

  const assets = {};
  if (existsSync(`demos/${name}/wasm`)) {
    execFileSync("go", ["build", "-trimpath", "-ldflags=-s -w", "-o", `${dir}/app.wasm`, `./demos/${name}/wasm`], {
      env: { ...process.env, GOOS: "js", GOARCH: "wasm", GOFLAGS: "-buildvcs=false" },
      stdio: "inherit",
    });
    copyFileSync(`${goroot}/lib/wasm/wasm_exec.js`, `${dir}/wasm_exec.js`);
    await build({
      entryPoints: [here("../tsappkit/src/wasmhost/worker.ts")],
      bundle: true,
      format: "iife",
      outfile: `${dir}/worker.js`,
      logLevel: "warning",
    });
    for (const [key, file] of [["wasm", "app.wasm"], ["exec", "wasm_exec.js"], ["worker", "worker.js"]]) {
      assets[key] = `${file}?v=${version(`${dir}/${file}`)}`;
    }
  }

  const islands = existsSync(`demos/${name}/page`);
  const result = await build({
    entryPoints: [`demos/${name}/main.ts`],
    bundle: true,
    format: "esm",
    target: "es2022",
    ...(islands
      ? { splitting: true, outdir: dir, entryNames: "[name]", chunkNames: "chunks/[name]-[hash]", metafile: true }
      : { outfile: `${dir}/main.js` }),
    minify: true,
    define: { DEMO_ASSETS: JSON.stringify(assets) },
    plugins: [oneCopy, solidPlugin()],
    logLevel: "warning",
  });
  const main = `main.js?v=${version(`${dir}/main.js`)}`;
  if (islands) {
    // The one-copy rule, held: one solid-js, and no tsappkit from npm.
    const inputs = Object.keys(result.metafile.inputs);
    const solids = new Set(inputs.filter((i) => i.includes("/solid-js/")).map((i) => i.split("/solid-js/")[0]));
    if (solids.size > 1 || inputs.some((i) => i.includes("@panyam/tsappkit/"))) {
      throw new Error(`demos/${name}: bundled ${solids.size} copies of solid-js, or tsappkit from npm`);
    }
    const meta = `${dir}.meta.json`;
    writeFileSync(meta, JSON.stringify(result.metafile));
    const html = execFileSync("go", ["run", `./demos/${name}/page`, "-meta", meta, "-outdir", dir, "-main", main], {
      env: { ...process.env, GOFLAGS: "-buildvcs=false" },
      encoding: "utf8",
      stdio: ["ignore", "pipe", "inherit"],
    });
    rmSync(meta);
    writeFileSync(`${dir}/index.html`, html);
    continue;
  }
  const html = readFileSync(`demos/${name}/index.html`, "utf8");
  if (!html.includes('src="main.js"')) throw new Error(`demos/${name}/index.html doesn't load main.js`);
  writeFileSync(`${dir}/index.html`, html.replace('src="main.js"', `src="${main}"`));
}
