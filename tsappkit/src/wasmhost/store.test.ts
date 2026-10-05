import { describe, expect, it } from "vitest";
import { opfsStore, workerStore, type StoreDirectory, type StoreFile } from "./store";

// An in-memory stand-in for an OPFS directory: files hold bytes, a SyncAccessHandle writes them,
// and move renames, as Chromium's do. reads records what a get could have seen at each step.
function fakeDir(opts: { move?: boolean; sync?: boolean } = {}) {
  const files = new Map<string, Uint8Array>();
  const notFound = () => Object.assign(new Error("not found"), { name: "NotFoundError" });
  const handle = (name: string): StoreFile => ({
    async getFile() {
      const b = files.get(name);
      if (!b) throw notFound();
      return { arrayBuffer: async () => b.slice().buffer };
    },
    ...(opts.sync !== false && {
      async createSyncAccessHandle() {
        let buf = files.get(name) ?? new Uint8Array();
        return {
          truncate: (n: number) => void (buf = buf.slice(0, n)),
          write: (b: Uint8Array) => ((buf = b.slice()), b.length),
          flush: () => void files.set(name, buf),
          close: () => {},
        };
      },
    }),
    ...(opts.sync === false && {
      async createWritable() {
        let buf = new Uint8Array();
        return { write: async (b: Uint8Array) => void (buf = b.slice()), close: async () => void files.set(name, buf) };
      },
    }),
    ...(opts.move !== false && {
      async move(_dir: StoreDirectory, to: string) {
        files.set(to, files.get(name)!);
        files.delete(name);
      },
    }),
  });
  const dir: StoreDirectory = {
    async getFileHandle(name, o) {
      if (!files.has(name)) {
        if (!o?.create) throw notFound();
        files.set(name, new Uint8Array());
      }
      return handle(name);
    },
    async removeEntry(name) {
      files.delete(name);
    },
  };
  return { dir, files };
}

describe("opfsStore", () => {
  it("gets back what was put, null for a missing key, and the newest of two puts", async () => {
    const { dir, files } = fakeDir();
    const s = opfsStore(dir);
    expect(await s.get("k")).toBeNull();
    await s.put("k", new Uint8Array([1, 2, 3]));
    expect(Array.from((await s.get("k"))!)).toEqual([1, 2, 3]);
    await s.put("k", new Uint8Array([9]));
    expect(Array.from((await s.get("k"))!)).toEqual([9]);
    expect([...files.keys()]).toEqual(["k"]);
  });

  it("never leaves an empty or partial file under the key while a put is writing", async () => {
    const { dir, files } = fakeDir();
    const seen: (number | undefined)[] = [];
    const watched: StoreDirectory = {
      getFileHandle: async (name, o) => {
        seen.push(files.get("k")?.length);
        return dir.getFileHandle(name, o);
      },
      removeEntry: (n) => dir.removeEntry(n),
    };
    await opfsStore(watched).put("k", new Uint8Array([1, 2, 3]));
    expect(seen.every((n) => n === undefined)).toBe(true);
    expect(files.get("k")?.length).toBe(3);
  });

  it("writes in place where files can't be moved, and with createWritable where there's no SyncAccessHandle", async () => {
    for (const opts of [{ move: false }, { sync: false }]) {
      const { dir, files } = fakeDir(opts);
      const s = opfsStore(dir);
      await s.put("k", new Uint8Array([4, 5]));
      expect(Array.from((await s.get("k"))!)).toEqual([4, 5]);
      expect([...files.keys()]).toEqual(["k"]);
    }
  });

  it("rethrows a get error that isn't a missing file", async () => {
    const s = opfsStore({
      getFileHandle: async () => {
        throw Object.assign(new Error("denied"), { name: "SecurityError" });
      },
      removeEntry: async () => {},
    });
    await expect(s.get("k")).rejects.toThrow("denied");
  });
});

describe("workerStore", () => {
  it("is undefined without navigator.storage.getDirectory, and opens wasmhost/<ns> once", async () => {
    expect(workerStore("ns", undefined)).toBeUndefined();
    expect(workerStore("ns", {})).toBeUndefined();

    const { dir } = fakeDir();
    const opened: string[] = [];
    const node = (path: string) => ({
      async getDirectoryHandle(name: string) {
        opened.push(`${path}/${name}`);
        return path === "/wasmhost" ? dir : node(`${path}/${name}`);
      },
    });
    const s = workerStore("state", { getDirectory: async () => node("") })!;
    await s.put("k", new Uint8Array([7]));
    expect(Array.from((await s.get("k"))!)).toEqual([7]);
    expect(opened).toEqual(["/wasmhost", "/wasmhost/state"]);
  });
});
