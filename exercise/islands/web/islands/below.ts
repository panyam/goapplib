import type { LCMComponent } from "../../../../tsappkit/src";
import { state } from "../record";

state().loaded.push("below");

export default function below(el: HTMLElement): LCMComponent {
  el.replaceChildren(Object.assign(document.createElement("p"), { textContent: "below mounted" }));
  el.dataset.mountedMs = String(Math.round(performance.now()));
  el.dataset.mounted = "below";
  state().mounted.push("below");
  return { performLocalInit: () => [], setupDependencies() {}, activate() {}, deactivate() {} };
}
