// The wasmhost-files demo: exercise/wasmhost's FilesService, a Go Connect service, running as wasm
// in a Web Worker. The page starts the worker, mounts files into it, and calls the service through
// its generated client with the worker's fetch as the transport. On load it runs each step once and
// checks it; the buttons do the same on demand.
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { addFiles, filesFromDrop, mountFiles, startWorker, workerFetch } from "../../../tsappkit/src/wasmhost";
import { FilesService } from "../../../exercise/wasmhost/web/gen/files/v1/files_pb";
import { assets, failed, ready } from "../_lib/frame";

const $ = (id: string) => document.getElementById(id)!;
const enc = new TextEncoder();

function step(text: string, ok = true) {
  const li = document.createElement("li");
  li.textContent = (ok ? "" : "Failed: ") + text;
  $("steps").append(li);
  if (!ok) throw new Error(text);
}

// While the worker spins, the page's own timer should keep firing every few ms. A service on the
// page's thread would hold it off for the whole spin.
async function spinAndWatch(client: ReturnType<typeof createClient<typeof FilesService>>, ms: number) {
  let last = performance.now();
  let maxGap = 0;
  const timer = setInterval(() => {
    const now = performance.now();
    maxGap = Math.max(maxGap, now - last);
    last = now;
  }, 10);
  try {
    const res = await client.spin({ ms });
    return { iterations: res.iterations, maxGap: Math.round(maxGap) };
  } finally {
    clearInterval(timer);
  }
}

async function run() {
  const t0 = performance.now();
  const worker = await startWorker({ ...assets, ns: "files" });
  step(`Started the worker and loaded the Go wasm in ${Math.round(performance.now() - t0)} ms.`);

  const client = createClient(FilesService, createConnectTransport({ baseUrl: location.origin, fetch: workerFetch(worker) }));
  const text = () => ($("text") as HTMLTextAreaElement).value;

  await mountFiles(worker, "docs", { "hello.txt": enc.encode(text()) });
  const first = await client.readFile({ path: "docs/hello.txt" });
  step(`Mounted docs/hello.txt, and ReadFile over Connect answered ${JSON.stringify(first.content)}.`, first.content === text());

  await addFiles(worker, "docs", { "second.txt": enc.encode("added later") });
  const [a, b] = await Promise.all([client.readFile({ path: "docs/hello.txt" }), client.readFile({ path: "docs/second.txt" })]);
  step(`Added docs/second.txt with addFiles, and docs/hello.txt is still there.`, a.content === text() && b.content === "added later");

  const spun = await spinAndWatch(client, 1000);
  step(
    `Kept the worker busy for 1 s (${spun.iterations} loop iterations); the page's timer never waited more than ${spun.maxGap} ms.`,
    spun.maxGap < 250,
  );
  ready();

  const out = (s: string) => ($("out").textContent = s);
  const read = $("read") as HTMLButtonElement;
  const spin = $("spin") as HTMLButtonElement;
  read.disabled = spin.disabled = false;
  read.onclick = async () => {
    await mountFiles(worker, "docs", { "hello.txt": enc.encode(text()) });
    const res = await client.readFile({ path: "docs/hello.txt" });
    out(`ReadFile("docs/hello.txt") → ${JSON.stringify(res.content)}`);
  };
  spin.onclick = async () => {
    spin.disabled = true;
    out("The worker is spinning; the dot keeps pulsing, since the page's thread is free.");
    const r = await spinAndWatch(client, 2000);
    out(`Spun ${r.iterations} iterations in 2 s; the page's timer never waited more than ${r.maxGap} ms.`);
    spin.disabled = false;
  };
  const drop = $("drop");
  drop.ondragover = (e) => {
    e.preventDefault();
    drop.classList.add("over");
  };
  drop.ondragleave = () => drop.classList.remove("over");
  drop.ondrop = async (e) => {
    e.preventDefault();
    drop.classList.remove("over");
    const files = await filesFromDrop(e.dataTransfer!);
    const paths = Object.keys(files).sort();
    if (!paths.length) return void out("Nothing to mount.");
    await mountFiles(worker, "dropped", files);
    const res = await client.readFile({ path: `dropped/${paths[0]}` });
    out(`Mounted ${paths.length} file(s) as dropped/. ReadFile("dropped/${paths[0]}") → ${JSON.stringify(res.content.slice(0, 300))}`);
  };
}

run().catch(failed);
