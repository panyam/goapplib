// A tour over a small app that reports what the reader did on an EventBus. The log under the app
// shows those events, so you can see which one moved each step along, and how the tour ended.
import { EventBus, Tour, type TourStep } from "../../../tsappkit/src";
import { ready } from "../_lib/frame";

interface AppEvents {
  checksRun: void;
  findingSelected: { id: string };
}

const bus = new EventBus();
const $ = <T extends HTMLElement = HTMLElement>(sel: string) => document.querySelector<T>(sel)!;
const items = () => document.querySelectorAll<HTMLLIElement>("#findings li");
const events: string[] = [];
const show = (line: string) => {
  events.push(line);
  $("#events").textContent = events.slice(-4).join("\n");
};

const app = {
  runChecks() {
    $("#findings").hidden = false;
    show("app: checksRun");
    bus.emit("checksRun");
  },
  select(id: string) {
    items().forEach((li) => li.classList.toggle("selected", li.dataset.id === id));
    show(`app: findingSelected ${id}`);
    bus.emit("findingSelected", { id });
  },
};
$("#run").addEventListener("click", () => app.runChecks());
items().forEach((li) => li.addEventListener("click", () => app.select(li.dataset.id!)));

const steps: TourStep<AppEvents>[] = [
  { id: "run", text: "Run the checks. (Show me does it for you, through the same app call.)", target: () => $("#run"),
    advanceOn: { event: "checksRun" }, doItForMe: () => app.runChecks() },
  { id: "list", text: "These are the findings, one row each.", target: () => $("#findings") },
  { id: "pick", text: "Click F2. Click another one and the tour ends, leaving that finding selected.", target: items,
    advanceOn: { event: "findingSelected", where: (d) => d.id === "F2" }, offPath: { event: "findingSelected" } },
  { id: "help", text: "Hover the ? for how findings are sorted.", target: () => $("#help"), advanceOn: { dom: "pointerenter" } },
];

const tour = new Tour<AppEvents>(steps, {
  onStep: (s) => show(`tour: step ${s.id}`),
  onEnd: (reason, s) => show(`tour: ended (${reason}) on ${s.id}`),
});
tour.attach(bus);
$("#start").addEventListener("click", () => {
  $("#findings").hidden = true;
  items().forEach((li) => li.classList.remove("selected"));
  tour.start();
});
ready();
