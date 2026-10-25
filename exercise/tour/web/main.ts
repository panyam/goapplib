// A fixture app for the tour's browser spec: it reports what the reader did on an EventBus, the way
// an app written for tours does, and window.fixture lets run.mjs start tours and read what happened.
import { EventBus } from "../../../tsappkit/src/EventBus";
import { Tour, type TourEndReason, type TourStep } from "../../../tsappkit/src/tour/Tour";

interface AppEvents {
  checksRun: void;
  rowSelected: { row: number };
}

const bus = new EventBus();
const $ = <T extends HTMLElement = HTMLElement>(sel: string) => document.querySelector<T>(sel)!;
const rows = () => document.querySelectorAll<HTMLTableRowElement>("#parts tr");

const app = {
  checksRun: 0,
  otherClicks: 0,
  selected: -1,
  runChecks() {
    app.checksRun++;
    $("#status").textContent = `Checks run ${app.checksRun}x`;
    bus.emit("checksRun");
  },
  select(row: number) {
    app.selected = row;
    rows().forEach((r, i) => r.classList.toggle("selected", i === row));
    bus.emit("rowSelected", { row });
  },
  move() {
    $("#mover").style.left = "300px";
  },
};

$("#run").addEventListener("click", () => app.runChecks());
$("#other").addEventListener("click", () => app.otherClicks++);
rows().forEach((r, i) => r.addEventListener("click", () => app.select(i)));
$("#log").innerHTML = Array.from({ length: 30 }, (_, i) => `<p>Finding ${i + 1}</p>`).join("");

const steps: TourStep<AppEvents>[] = [
  { id: "run", text: "Run the design checks.", target: () => $("#run"), advanceOn: { event: "checksRun" }, doItForMe: () => app.runChecks() },
  {
    id: "rows",
    title: "Parts",
    text: "Click the first part in the table.",
    target: rows,
    advanceOn: { event: "rowSelected", where: (d) => d.row === 0 },
    offPath: { event: "rowSelected" },
  },
  { id: "hover", text: "Hover the ? for the part's footprint.", target: () => $("#tip"), advanceOn: { dom: "pointerenter" } },
  { id: "mover", text: "This box moves, and its hole follows it.", target: () => $("#mover") },
  { id: "log", text: "Scroll the findings.", target: () => $("#log") },
];

const log: { step?: string; end?: TourEndReason; at?: string }[] = [];
const tour = new Tour<AppEvents>(steps, {
  onStep: (s) => log.push({ step: s.id }),
  onEnd: (reason, s) => log.push({ end: reason, at: s.id }),
});
tour.attach(bus);

(window as any).fixture = {
  app,
  log,
  start: (at = 0) => tour.start(at),
  step: () => tour.step?.id ?? null,
};
