// The islands-presenter demo: one EditorPresenter, built in makeContext, drives two Solid islands
// through a Port each. The history island loads "visible", below a spacer, so the check makes picks
// before it mounts and then scrolls to it.
import { BasePage, IslandPage, type EventBus, type LCMComponent, type Registry } from "../../../tsappkit/src";
import { failed, ready } from "../_lib/frame";
import history from "./islands/history";
import tools from "./islands/tools";
import { EditorPresenter } from "./presenter";

type Ctx = { editor: EditorPresenter };
let editor: EditorPresenter | undefined;

class DemoPage extends IslandPage<Ctx> {
  protected registry(): Registry<Ctx, HTMLElement, LCMComponent, EventBus> {
    return { tools, history };
  }
  protected makeContext() {
    editor = new EditorPresenter();
    return { editor };
  }
}
BasePage.loadAfterPageLoaded("demoPage", DemoPage, "DemoPage");

const $ = (id: string) => document.getElementById(id)!;
const wait = (ms: number) => new Promise((ok) => setTimeout(ok, ms));
const until = async (cond: () => boolean) => {
  for (let i = 0; i < 100 && !cond(); i++) await wait(50);
  return cond();
};
function step(text: string, ok: boolean) {
  const li = document.createElement("li");
  li.textContent = (ok ? "" : "Failed: ") + text;
  $("steps").append(li);
  if (!ok) throw new Error(text);
}

async function check() {
  const slot = (name: string) => document.querySelector<HTMLElement>(`[data-slot="${name}"]`)!;
  step("The tools island mounted, and history, below the fold, didn't.", (await until(() => slot("tools").dataset.mounted === "tools")) && !slot("history").dataset.mounted);
  $("pick-green").click();
  $("pick-blue").click();
  step("Picks in tools reached the presenter and came back to tools.", $("current").textContent === "blue");
  slot("history").scrollIntoView();
  step("Scrolled into view, history mounted and showed the picks made before it existed.", (await until(() => slot("history").dataset.mounted === "history")) && $("picks").textContent === "green, blue");
  $("pick-red").click();
  step("A pick in tools now shows in history too, through the presenter.", $("picks").textContent === "green, blue, red");
  step("Each port has exactly one view attached.", editor?.tools.size === 1 && editor?.history.size === 1);
  window.scrollTo(0, 0);
  ready();
}
check().catch(failed);
