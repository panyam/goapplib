// Turning what a user drops or picks into the {path: bytes} map mountFiles takes. The bytes are
// read here, before any request, because the Go side can't wait on File.arrayBuffer() mid-request.
import type { Files } from "./protocol";

// The parts of the File and Directory Entries API used here. Chromium, Firefox and Safari all ship
// it under the webkit prefix.
interface Entry {
  isFile: boolean;
  isDirectory: boolean;
  fullPath: string;
}
interface FileEntry extends Entry {
  file(ok: (f: File) => void, err: (e: unknown) => void): void;
}
interface DirectoryEntry extends Entry {
  createReader(): { readEntries(ok: (es: Entry[]) => void, err: (e: unknown) => void): void };
}

/**
 * Reads a drop into files keyed by path. A dropped folder keeps its own name as the first path
 * element ("design/board.edn"), and dropped loose files sit at the top. Entries the browser can't
 * read as files or folders are skipped.
 */
export async function filesFromDrop(dt: DataTransfer): Promise<Files> {
  const entries: Entry[] = [];
  const loose: File[] = [];
  for (const item of Array.from(dt.items)) {
    if (item.kind !== "file") continue;
    const entry = (item as DataTransferItem & { webkitGetAsEntry?: () => Entry | null }).webkitGetAsEntry?.();
    if (entry) entries.push(entry);
    else {
      const f = item.getAsFile();
      if (f) loose.push(f);
    }
  }
  const out: Files = {};
  await Promise.all([
    ...entries.map((e) => readEntry(e, out)),
    ...loose.map(async (f) => {
      out[f.name] = new Uint8Array(await f.arrayBuffer());
    }),
  ]);
  return out;
}

/**
 * Reads an `<input type="file" webkitdirectory>` (or `multiple`) selection into files keyed by
 * path. Folder picks use each file's webkitRelativePath, which starts with the folder's name.
 */
export async function filesFromFileList(list: FileList | File[]): Promise<Files> {
  const out: Files = {};
  await Promise.all(
    Array.from(list).map(async (f) => {
      out[f.webkitRelativePath || f.name] = new Uint8Array(await f.arrayBuffer());
    }),
  );
  return out;
}

async function readEntry(entry: Entry, out: Files): Promise<void> {
  if (entry.isFile) {
    const f = await new Promise<File>((ok, err) => (entry as FileEntry).file(ok, err));
    out[entry.fullPath.replace(/^\/+/, "")] = new Uint8Array(await f.arrayBuffer());
    return;
  }
  if (!entry.isDirectory) return;
  const reader = (entry as DirectoryEntry).createReader();
  // readEntries returns a batch at a time (100 in Chromium) and an empty batch at the end.
  for (;;) {
    const batch = await new Promise<Entry[]>((ok, err) => reader.readEntries(ok, err));
    if (batch.length === 0) return;
    await Promise.all(batch.map((e) => readEntry(e, out)));
  }
}
