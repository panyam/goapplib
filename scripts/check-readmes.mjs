#!/usr/bin/env node
// Each TS package's README is its npm page. Fails when a README doesn't name one
// of its package's entry points (package.json "exports", less "./dist/*"), when
// a link to the docs site or to a file on main points at a page, heading or file
// that doesn't exist, or when its license isn't package.json's. CI runs it on
// every PR.
//
//   scripts/check-readmes.mjs
import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "..");
const site = "https://panyam.github.io/goapplib/";
const repo = "https://github.com/panyam/goapplib/blob/main/";

const slug = (heading) =>
  heading.toLowerCase().replace(/[^a-z0-9 -]/g, "").trim().replace(/ /g, "-");

function sitePage(path) {
  const base = join(root, "docsite/content", path.replace(/\/$/, ""));
  for (const f of [base + ".md", join(base, "index.md")]) if (existsSync(f)) return f;
  return null;
}

const problems = [];
for (const dir of ["tsappkit", "tsappkit-solid"]) {
  const pkg = JSON.parse(readFileSync(join(root, dir, "package.json"), "utf8"));
  const file = join(root, dir, "README.md");
  if (!existsSync(file)) {
    problems.push(`${dir}/README.md is missing`);
    continue;
  }
  const readme = readFileSync(file, "utf8");

  for (const key of Object.keys(pkg.exports ?? { ".": "" })) {
    if (key === "./dist/*") continue;
    const name = pkg.name + key.slice(1).replace(/\*$/, "");
    if (!readme.includes(name)) problems.push(`${dir}/README.md doesn't name ${name}`);
  }

  for (const [, url] of readme.matchAll(/\]\((https:\/\/[^)\s]+)\)/g)) {
    if (url.startsWith(site)) {
      const [path, anchor] = url.slice(site.length).split("#");
      const page = sitePage(path);
      if (!page) {
        problems.push(`${dir}/README.md links ${url}, and docsite/content has no such page`);
      } else if (anchor) {
        const headings = [...readFileSync(page, "utf8").matchAll(/^#+ (.+)$/gm)].map((m) => slug(m[1]));
        if (!headings.includes(anchor)) problems.push(`${dir}/README.md links ${url}, and the page has no such heading`);
      }
    } else if (url.startsWith(repo) && !existsSync(join(root, url.slice(repo.length).split("#")[0]))) {
      problems.push(`${dir}/README.md links ${url}, and the repo has no such file`);
    }
  }

  const license = readme.match(/^## License\s+(\S+)/m)?.[1];
  if (license !== pkg.license) problems.push(`${dir}/README.md says license ${license}; package.json says ${pkg.license}`);
}

for (const p of problems) console.error(p);
if (problems.length) process.exit(1);
console.log("READMEs name every entry point, and their links resolve");
