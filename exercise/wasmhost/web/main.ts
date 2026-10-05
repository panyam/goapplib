// The exercise page. It does what an app would: start the worker, push files in, and talk to the
// service through its generated Connect client with the worker's fetch as the transport. Each step
// lands in window.exercise for run.mjs to check, and in the page for a person to read.
import { createClient } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { addFiles, mountFiles, startWorker, workerFetch, workerMemory } from "../../../tsappkit/src/wasmhost";
import { FilesService } from "./gen/files/v1/files_pb";

interface Step {
  name: string;
  ok: boolean;
  detail: string;
}

const state = { steps: [] as Step[], done: false };
(window as unknown as { exercise: typeof state }).exercise = state;

function record(name: string, ok: boolean, detail: string) {
  state.steps.push({ name, ok, detail });
  const li = document.createElement("li");
  li.textContent = `${ok ? "PASS" : "FAIL"} ${name}: ${detail}`;
  document.getElementById("steps")!.append(li);
}

const MOUNTED = "hello from a mounted file";
const ADDED = "added later";

async function run() {
  const t0 = performance.now();
  const worker = await startWorker({ worker: "worker.js", wasm: "files.wasm", exec: "wasm_exec.js", ns: "files" });
  record("start", true, `worker ready in ${Math.round(performance.now() - t0)} ms`);

  await mountFiles(worker, "docs", { "hello.txt": new TextEncoder().encode(MOUNTED) });
  record("mount", true, "mounted docs/hello.txt");

  const client = createClient(
    FilesService,
    createConnectTransport({ baseUrl: location.origin, fetch: workerFetch(worker) }),
  );
  const res = await client.readFile({ path: "docs/hello.txt" });
  record("read", res.content === MOUNTED, `ReadFile returned ${JSON.stringify(res.content)}`);

  // Adding a file must keep the one already mounted, which a replacing mount would drop.
  await addFiles(worker, "docs", { "second.txt": new TextEncoder().encode(ADDED) });
  const [first, second] = await Promise.all([
    client.readFile({ path: "docs/hello.txt" }),
    client.readFile({ path: "docs/second.txt" }),
  ]);
  record(
    "add",
    first.content === MOUNTED && second.content === ADDED,
    `after addFiles, hello.txt=${JSON.stringify(first.content)} second.txt=${JSON.stringify(second.content)}`,
  );

  const bytes = await workerMemory(worker);
  record("memory", bytes > 0, `the engine holds ${(bytes / 1024 / 1024).toFixed(1)} MB of wasm memory`);

  // The worker spins for 2 s while the page counts animation frames. A host on the page's own
  // thread would get a handful; a worker leaves the page at its normal frame rate.
  let frames = 0;
  let counting = true;
  const tick = () => {
    frames++;
    if (counting) requestAnimationFrame(tick);
  };
  requestAnimationFrame(tick);
  const spin = await client.spin({ ms: 2000 });
  counting = false;
  record("responsive", frames >= 20, `${frames} frames drawn while the worker spun ${spin.iterations} iterations`);
}

run()
  .catch((err: unknown) => record("error", false, String(err)))
  .finally(() => {
    state.done = true;
  });
