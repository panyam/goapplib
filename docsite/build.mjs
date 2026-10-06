// Bundles each live demo in demos/<name>/ (an index.html and a main.ts) into static/demos/<name>/,
// which `make build` copies into the site. demos/_lib/ is the code they share, not a demo.
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

const here = (p) => fileURLToPath(new URL(p, import.meta.url));
const version = (file) => createHash("sha256").update(readFileSync(file)).digest("hex").slice(0, 12);

// A demo may import code from elsewhere in the repo (the exercises' generated Connect clients),
// whose imports would otherwise resolve from that directory's node_modules, if it has one, and
// bundle a second copy. Resolve them from this package instead.
const oneCopy = {
  name: "one-copy",
  setup(b) {
    b.onResolve({ filter: /^@(bufbuild|connectrpc)\// }, (args) => {
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

  await build({
    entryPoints: [`demos/${name}/main.ts`],
    bundle: true,
    format: "esm",
    target: "es2022",
    outfile: `${dir}/main.js`,
    minify: true,
    define: { DEMO_ASSETS: JSON.stringify(assets) },
    plugins: [oneCopy],
    logLevel: "warning",
  });
  const html = readFileSync(`demos/${name}/index.html`, "utf8");
  if (!html.includes('src="main.js"')) throw new Error(`demos/${name}/index.html doesn't load main.js`);
  writeFileSync(`${dir}/index.html`, html.replace('src="main.js"', `src="main.js?v=${version(`${dir}/main.js`)}"`));
}
