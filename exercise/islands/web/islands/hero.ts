import type { LCMComponent } from "../../../../tsappkit/src";
import { state } from "../record";

state().loaded.push("hero");

export function hero(el: HTMLElement): LCMComponent {
  el.replaceChildren(Object.assign(document.createElement("p"), { textContent: "hero mounted" }));
  el.dataset.mounted = "hero";
  state().mounted.push("hero");
  return { performLocalInit: () => [], setupDependencies() {}, activate() {}, deactivate() {} };
}
