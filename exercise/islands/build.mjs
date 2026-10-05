// Builds the exercise page: the same esbuild flags the islands mission has always used (splitting,
// so each lazy island is its own chunk, and a metafile for Go's modulepreload links), plus Solid's
// JSX for the below island, which is a real SolidIsland from ../../tsappkit-solid/src.
//
// tsappkit-solid's sources import @panyam/tsappkit and solid-js, and would find them in
// tsappkit-solid/node_modules: tsappkit 0.1.0 from npm and a second solid-js. Without the one-copy
// plugin below, below's chunk grows from about 28 KB to 87 KB, and Solid's effects come from a
// different copy than the root SolidIsland disposes. So both are pointed at one copy, tsappkit's
// own sources and this package's solid-js. run.mjs's one-copy check holds the build to that.
import { build } from "esbuild";
import { solidPlugin } from "esbuild-plugin-solid";
import { writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";

const here = (p) => fileURLToPath(new URL(p, import.meta.url));

const oneCopy = {
  name: "one-copy",
  setup(b) {
    b.onResolve({ filter: /^@panyam\/tsappkit$/ }, () => ({ path: here("../../tsappkit/src/index.ts") }));
    b.onResolve({ filter: /^solid-js(\/.*)?$/ }, (args) => {
      if (args.pluginData?.oneCopy) return undefined;
      return b.resolve(args.path, { kind: args.kind, resolveDir: here("."), pluginData: { oneCopy: true } });
    });
  },
};

const result = await build({
  entryPoints: ["web/main.ts"],
  bundle: true,
  splitting: true,
  format: "esm",
  outdir: "dist",
  entryNames: "[name]",
  chunkNames: "chunks/[name]-[hash]",
  metafile: true,
  logLevel: "warning",
  plugins: [oneCopy, solidPlugin()],
});
await writeFile("dist/meta.json", JSON.stringify(result.metafile));
