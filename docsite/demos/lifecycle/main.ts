// Starts the page the way BasePage.loadAfterPageLoaded does, then mounts a late component on a
// controller of its own. Each lifecycle call lands in the log, and the demo checks their order.
import { LifecycleController } from "../../../tsappkit/src";
import { failed, ready } from "../_lib/frame";
import { DemoPage, LateBadge, type Trace } from "./components";

const calls: string[] = [];
const log = document.getElementById("log")!;
const row = (component: string, call: string) => {
  const li = document.createElement("li");
  li.dataset.call = call.split(" ")[0];
  const b = document.createElement("b");
  b.textContent = component;
  li.append(b, ` ${call}`);
  log.append(li);
};
const trace: Trace = (component, call) => {
  calls.push(`${component} ${call}`);
  row(component, call);
};

const expected = [
  "page performLocalInit", "store performLocalInit", "counter performLocalInit", "button performLocalInit",
  "page setupDependencies", "store setupDependencies", "counter setupDependencies", "button setupDependencies",
  "page activate", "store activate", "counter activate", "counter got count 3", "store activate finished",
  "button activate",
  "badge performLocalInit", "badge setupDependencies", "badge activate",
];

try {
  const page = new DemoPage(trace);
  await new LifecycleController(page.eventBus).initializeFromRoot(page);
  row("--", "the page has started; mounting a late component");
  await new LifecycleController(page.eventBus).initializeFromRoot(new LateBadge(page.eventBus, page.store, trace));
  const off = expected.findIndex((c, i) => calls[i] !== c);
  if (off >= 0 || calls.length !== expected.length) failed(`call ${off + 1} was "${calls[off]}", expected "${expected[off]}"`);
  else ready();
} catch (err) {
  failed(err);
}
