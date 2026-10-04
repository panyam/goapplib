import type { LCMComponent } from "../../../../tsappkit/src";
import { state } from "../record";

state().loaded.push("narrow");

export function narrow(el: HTMLElement): LCMComponent {
  el.replaceChildren(Object.assign(document.createElement("p"), { textContent: "narrow mounted" }));
  el.dataset.mountedMs = String(Math.round(performance.now()));
  el.dataset.mounted = "narrow";
  state().mounted.push("narrow");
  return { performLocalInit: () => [], setupDependencies() {}, activate() {}, deactivate() {} };
}
