// The islands-solid demo: one lazy SolidIsland over a placeholder Go rendered in its slot. With
// ?slow in the URL its chunk waits 1.5 s, so a reader can see the placeholder; with ?islands,
// IslandPage draws its debug overlay. On load it checks the island took the slot over.
import { BasePage, IslandPage, lazy, type EventBus, type LCMComponent, type Registry } from "../../../tsappkit/src";
import { failed, ready } from "../_lib/frame";

const slow = new URLSearchParams(location.search).has("slow");
const wait = (ms: number) => new Promise((ok) => setTimeout(ok, ms));

class DemoPage extends IslandPage<Record<string, never>> {
  protected registry(): Registry<Record<string, never>, HTMLElement, LCMComponent, EventBus> {
    return {
      counter: lazy(async () => {
        if (slow) await wait(1500);
        return import("./islands/counter");
      }),
    };
  }
  protected makeContext() {
    return {};
  }
}
BasePage.loadAfterPageLoaded("demoPage", DemoPage, "DemoPage");

const $ = (id: string) => document.getElementById(id)!;
function step(text: string, ok: boolean) {
  const li = document.createElement("li");
  li.textContent = (ok ? "" : "Failed: ") + text;
  $("steps").append(li);
  if (!ok) throw new Error(text);
}

async function check() {
  const slot = document.querySelector<HTMLElement>('[data-slot="main"]')!;
  for (let i = 0; i < 100 && !slot.dataset.mounted; i++) await wait(50);
  step("The counter island mounted into its slot.", slot.dataset.mounted === "counter");
  step("It replaced the placeholder Go drew there, so the slot has one owner.", !slot.querySelector("[data-fallback]") && !!slot.querySelector("button"));
  ready();
}
check().catch(failed);

const reload = (q: string) => () => {
  location.search = q;
};
$("slow").onclick = reload("?slow");
$("overlay").onclick = reload("?islands");
