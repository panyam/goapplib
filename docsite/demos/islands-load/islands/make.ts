import type { LCMComponent } from "../../../../tsappkit/src";
import { record } from "../record";

// Each island replaces its slot's fallback with a line saying when it mounted.
export function island(name: string, why: string) {
  record(name, "loaded");
  return (el: HTMLElement): LCMComponent => {
    record(name, "mounted");
    el.replaceChildren(Object.assign(document.createElement("p"), { textContent: `${name} mounted ${why}.` }));
    el.dataset.mounted = name;
    return { performLocalInit: () => [], setupDependencies() {}, activate() {}, deactivate() {} };
  };
}
