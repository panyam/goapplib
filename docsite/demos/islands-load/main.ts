// The islands-load demo: an IslandPage over the spec Go rendered into this page, with four lazy
// islands, one per load strategy. Each island is its own chunk, downloaded when its strategy says.
// On load it checks what should and shouldn't have happened yet, then keeps the table live.
import { BasePage, IslandPage, lazy, type EventBus, type LCMComponent, type Registry } from "../../../tsappkit/src";
import { failed, ready } from "../_lib/frame";
import { onChange, timesOf } from "./record";

class DemoPage extends IslandPage<Record<string, never>> {
  protected registry(): Registry<Record<string, never>, HTMLElement, LCMComponent, EventBus> {
    return {
      hero: lazy(() => import("./islands/hero")),
      later: lazy(() => import("./islands/later")),
      below: lazy(() => import("./islands/below")),
      narrow: lazy(() => import("./islands/narrow")),
    };
  }
  protected makeContext() {
    return {};
  }
}
BasePage.loadAfterPageLoaded("demoPage", DemoPage, "DemoPage");

const $ = (id: string) => document.getElementById(id)!;
const spec = JSON.parse($("page-spec").textContent!) as { islands: { name: string; load?: string }[] };
const preloaded = (name: string) =>
  [...document.querySelectorAll<HTMLLinkElement>('link[rel="modulepreload"]')].some((l) => l.href.includes(`/chunks/${name}-`));
const ms = (t?: number) => (t === undefined ? "not yet" : `${t} ms`);

function draw() {
  $("table").replaceChildren(
    ...spec.islands.map((is) => {
      const tr = document.createElement("tr");
      const t = timesOf(is.name);
      const load = is.load ?? "eager";
      for (const cell of [is.name, load.split(":")[0], preloaded(is.name) ? "yes" : "no", ms(t.loaded), ms(t.mounted)]) {
        tr.append(Object.assign(document.createElement("td"), { textContent: cell, title: cell === "media" ? load : "" }));
      }
      return tr;
    }),
  );
}
draw();
onChange(draw);

function step(text: string, ok: boolean) {
  const li = document.createElement("li");
  li.textContent = (ok ? "" : "Failed: ") + text;
  $("steps").append(li);
  if (!ok) throw new Error(text);
}

async function check() {
  for (let i = 0; i < 100 && !(timesOf("hero").mounted && timesOf("later").mounted); i++) await new Promise((ok) => setTimeout(ok, 50));
  step("hero (eager) mounted at once, and Go preloaded its chunk.", !!timesOf("hero").mounted && preloaded("hero"));
  step("later (idle) mounted once the browser was idle, without a preload.", !!timesOf("later").mounted && !preloaded("later"));
  step("below (visible) is out of view in the box, so its code hasn't downloaded.", timesOf("below").loaded === undefined);
  const narrow = matchMedia("(max-width: 600px)").matches;
  step(
    narrow ? "narrow (media) mounted, since this frame is under 600 px wide." : "narrow (media) hasn't downloaded, since this frame is wider than 600 px.",
    narrow ? !!timesOf("narrow").mounted : timesOf("narrow").loaded === undefined,
  );
  ready();
}
check().catch(failed);

const frame = window.frameElement as HTMLElement | null;
const button = $("narrow") as HTMLButtonElement;
if (!frame) button.hidden = true;
if (matchMedia("(max-width: 600px)").matches) button.disabled = true;
button.onclick = () => {
  frame!.style.width = "400px";
  button.disabled = true;
};
