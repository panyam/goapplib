/**
 * The worker side of wasmhost's Cache (BrowserCache in Go): blobs by key in a directory of the
 * Origin Private File System, which outlives a reload. worker.ts installs it as
 * globalThis.wasmhostCache before the Go program starts.
 *
 * Only the parts of the File System Access types it uses are named here, so tests can hand it a
 * fake directory and the package doesn't depend on the DOM lib having them.
 */

/** What Go's BrowserCache calls. get resolves to null for a key with nothing under it. */
export interface BlobCache {
  get(key: string): Promise<Uint8Array | null>;
  put(key: string, bytes: Uint8Array): Promise<void>;
}

export interface CacheFile {
  getFile(): Promise<{ arrayBuffer(): Promise<ArrayBuffer> }>;
  createSyncAccessHandle?(): Promise<{
    truncate(size: number): void;
    write(buf: Uint8Array, opts?: { at: number }): number;
    flush(): void;
    close(): void;
  }>;
  createWritable?(): Promise<{ write(data: Uint8Array): Promise<void>; close(): Promise<void> }>;
  move?(dir: CacheDirectory, name: string): Promise<void>;
}

export interface CacheDirectory {
  getFileHandle(name: string, opts?: { create?: boolean }): Promise<CacheFile>;
  removeEntry(name: string): Promise<void>;
}

/**
 * A BlobCache over dir, one file per key. put writes a temporary file and moves it over the key
 * where the browser can move files, so get never sees half a blob; elsewhere it writes in place.
 * In a dedicated worker the write goes through a SyncAccessHandle, which every browser with OPFS
 * offers there.
 */
export function opfsCache(dir: CacheDirectory): BlobCache {
  return {
    async get(key) {
      let file: CacheFile;
      try {
        file = await dir.getFileHandle(key);
      } catch (err) {
        if (isNotFound(err)) return null;
        throw err;
      }
      return new Uint8Array(await (await file.getFile()).arrayBuffer());
    },
    async put(key, bytes) {
      const tmp = `.put-${key}-${Math.random().toString(36).slice(2)}`;
      const file = await dir.getFileHandle(tmp, { create: true });
      if (!file.move) {
        await dir.removeEntry(tmp).catch(() => {});
        await write(await dir.getFileHandle(key, { create: true }), bytes);
        return;
      }
      try {
        await write(file, bytes);
        await file.move(dir, key);
      } catch (err) {
        await dir.removeEntry(tmp).catch(() => {});
        throw err;
      }
    },
  };
}

async function write(file: CacheFile, bytes: Uint8Array) {
  if (file.createSyncAccessHandle) {
    const h = await file.createSyncAccessHandle();
    try {
      h.truncate(0);
      h.write(bytes, { at: 0 });
      h.flush();
    } finally {
      h.close();
    }
  } else if (file.createWritable) {
    const w = await file.createWritable();
    await w.write(bytes);
    await w.close();
  } else {
    throw new Error("this browser can't write to the origin private file system");
  }
}

function isNotFound(err: unknown): boolean {
  return typeof err === "object" && err !== null && (err as { name?: unknown }).name === "NotFoundError";
}

/**
 * The cache for namespace ns under wasmhost/<ns>/ in the origin's private file system, or undefined
 * where there's none (navigator.storage.getDirectory missing). The directory is opened on first use.
 */
export function workerCache(ns: string, storage: { getDirectory?: () => Promise<unknown> } | undefined): BlobCache | undefined {
  if (!storage?.getDirectory) return undefined;
  const getDirectory = storage.getDirectory.bind(storage);
  let dir: Promise<BlobCache> | undefined;
  const open = () =>
    (dir ??= (async () => {
      const root = (await getDirectory()) as { getDirectoryHandle(n: string, o: { create: boolean }): Promise<unknown> };
      const base = (await root.getDirectoryHandle("wasmhost", { create: true })) as typeof root;
      return opfsCache((await base.getDirectoryHandle(ns, { create: true })) as CacheDirectory);
    })());
  return {
    get: async (key) => (await open()).get(key),
    put: async (key, bytes) => (await open()).put(key, bytes),
  };
}
