import { describe, expect, it } from "vitest";
import { filesFromDrop, filesFromFileList } from "./drop";

const file = (name: string, text: string, rel = "") => {
  const f = new File([text], name);
  if (rel) Object.defineProperty(f, "webkitRelativePath", { value: rel });
  return f;
};

const fileEntry = (fullPath: string, text: string) => ({
  isFile: true,
  isDirectory: false,
  fullPath,
  file: (ok: (f: File) => void) => ok(file(fullPath.split("/").pop()!, text)),
});

// readEntries hands back one child per call, then an empty batch, so a reader that stops after the
// first batch misses files.
const dirEntry = (fullPath: string, children: object[]) => ({
  isFile: false,
  isDirectory: true,
  fullPath,
  createReader: () => {
    const left = [...children];
    return { readEntries: (ok: (es: object[]) => void) => ok(left.length ? [left.shift()!] : []) };
  },
});

const drop = (items: object[]) => ({ items }) as unknown as DataTransfer;
const asText = (files: Record<string, Uint8Array>) =>
  Object.fromEntries(Object.entries(files).map(([k, v]) => [k, new TextDecoder().decode(v)]));

describe("filesFromDrop", () => {
  it("reads a dropped folder recursively, keeping its name, plus loose files", async () => {
    const folder = dirEntry("/design", [
      fileEntry("/design/board.edn", "b"),
      dirEntry("/design/sheets", [fileEntry("/design/sheets/p1.edn", "p1"), fileEntry("/design/sheets/p2.edn", "p2")]),
    ]);
    const out = await filesFromDrop(
      drop([
        { kind: "file", webkitGetAsEntry: () => folder },
        { kind: "file", webkitGetAsEntry: () => null, getAsFile: () => file("notes.txt", "n") },
        { kind: "string" },
      ]),
    );
    expect(asText(out)).toEqual({
      "design/board.edn": "b",
      "design/sheets/p1.edn": "p1",
      "design/sheets/p2.edn": "p2",
      "notes.txt": "n",
    });
  });
});

describe("filesFromFileList", () => {
  it("keys a folder pick by relative path and a plain pick by name", async () => {
    const out = await filesFromFileList([file("a.txt", "a", "proj/src/a.txt"), file("b.txt", "b")]);
    expect(asText(out)).toEqual({ "proj/src/a.txt": "a", "b.txt": "b" });
  });
});
