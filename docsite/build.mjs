// Bundles each live demo in demos/<name>/ (an index.html and a main.ts) into static/demos/<name>/,
// which `make build` copies into the site. demos/_lib/ is the code they share, not a demo.
// index.html's main.js is versioned by a hash of the bundle, so a browser holding an older copy from
// Pages' cache fetches the new one.
import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { build } from "esbuild";

const out = "static/demos";
rmSync(out, { recursive: true, force: true });
for (const name of readdirSync("demos").filter((n) => !n.startsWith("_"))) {
  const dir = `${out}/${name}`;
  mkdirSync(dir, { recursive: true });
  await build({
    entryPoints: [`demos/${name}/main.ts`],
    bundle: true,
    format: "esm",
    target: "es2022",
    outfile: `${dir}/main.js`,
    logLevel: "warning",
  });
  const hash = createHash("sha256").update(readFileSync(`${dir}/main.js`)).digest("hex").slice(0, 12);
  const html = readFileSync(`demos/${name}/index.html`, "utf8");
  if (!html.includes('src="main.js"')) throw new Error(`demos/${name}/index.html doesn't load main.js`);
  writeFileSync(`${dir}/index.html`, html.replace('src="main.js"', `src="main.js?v=${hash}"`));
}
