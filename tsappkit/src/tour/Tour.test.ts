import { afterEach, describe, expect, it, vi } from "vitest";
import { EventBus } from "../EventBus";
import { Tour, type TourEndReason, type TourStep } from "./Tour";

interface Ev {
  checksRun: void;
  cellSelected: { table: string; row: number };
}

const ended = () => {
  const log: [TourEndReason, string][] = [];
  return { log, onEnd: (r: TourEndReason, s: TourStep<Ev>) => log.push([r, s.id]) };
};

const steps: TourStep<Ev>[] = [
  { id: "run", text: "Run the checks.", advanceOn: { event: "checksRun" } },
  {
    id: "cell",
    text: "Click the first row.",
    advanceOn: { event: "cellSelected", where: (d) => d.row === 0 },
    offPath: { event: "cellSelected" },
  },
  { id: "look", text: "These are the findings." },
];

let tour: Tour<Ev> | undefined;
afterEach(() => {
  tour?.end();
  tour = undefined;
  document.body.replaceChildren();
});

const panel = () => document.querySelector(".tsappkit-tour-panel");
const button = (cls: string) => document.querySelector<HTMLButtonElement>(`.tsappkit-tour-${cls}`);

describe("Tour", () => {
  it("advances on the app event a step names, and only when its payload matches", () => {
    const e = ended();
    tour = new Tour<Ev>(steps, { onEnd: e.onEnd });
    tour.start();
    expect(tour.step?.id).toBe("run");
    tour.notify("cellSelected", { table: "t", row: 0 });
    expect(tour.step?.id).toBe("run");
    tour.notify("checksRun");
    expect(tour.step?.id).toBe("cell");
    expect(document.querySelector(".tsappkit-tour")?.getAttribute("data-tour-step")).toBe("cell");
    tour.notify("cellSelected", { table: "t", row: 0 });
    expect(tour.step?.id).toBe("look");
    expect(e.log).toEqual([]);
  });

  it("ends as offPath when a watched event doesn't match, and the overlay goes", () => {
    const e = ended();
    tour = new Tour<Ev>(steps, { onEnd: e.onEnd });
    tour.start(1);
    tour.notify("cellSelected", { table: "t", row: 1 });
    expect(tour.running).toBe(false);
    expect(e.log).toEqual([["offPath", "cell"]]);
    expect(document.querySelector(".tsappkit-tour")).toBeNull();
  });

  it("keeps waiting off path when the step says stay", () => {
    tour = new Tour<Ev>([{ ...steps[1], onOffPath: "stay" }]);
    tour.start();
    tour.notify("cellSelected", { table: "t", row: 1 });
    expect(tour.step?.id).toBe("cell");
    tour.notify("cellSelected", { table: "t", row: 0 });
    expect(tour.running).toBe(false);
  });

  it("ignores events no step on it watches", () => {
    tour = new Tour<Ev>([steps[0], steps[2]]);
    tour.start(1);
    tour.notify("checksRun");
    expect(tour.step?.id).toBe("look");
  });

  it("shows Next only on a step with no action, and Done on the last", () => {
    const e = ended();
    tour = new Tour<Ev>(steps, { onEnd: e.onEnd });
    tour.start();
    expect(button("next")).toBeNull();
    tour.start(2);
    expect(button("next")?.textContent).toBe("Done");
    button("next")!.click();
    expect(e.log).toEqual([["done", "look"]]);
  });

  it("ends on Esc and on the close button, as cancelled", () => {
    const e = ended();
    tour = new Tour<Ev>(steps, { onEnd: e.onEnd });
    tour.start();
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    expect(tour.running).toBe(false);
    tour.start(2);
    button("close")!.click();
    expect(e.log).toEqual([
      ["cancelled", "run"],
      ["cancelled", "look"],
    ]);
  });

  it("advances after Show me only through the app's own event", () => {
    const bus = new EventBus();
    const act = vi.fn();
    tour = new Tour<Ev>([{ ...steps[0], doItForMe: act }, steps[2]]);
    tour.attach(bus);
    tour.start();
    expect(button("showme")?.textContent).toBe("Show me");
    button("showme")!.click();
    expect(act).toHaveBeenCalledOnce();
    expect(tour.step?.id).toBe("run");

    tour.end();
    tour = new Tour<Ev>([{ ...steps[0], doItForMe: () => bus.emit("checksRun") }, steps[2]]);
    tour.attach(bus);
    tour.start();
    button("showme")!.click();
    expect(tour.step?.id).toBe("look");
  });

  it("subscribes to the events its steps name, until detached", () => {
    const bus = new EventBus();
    const on = vi.spyOn(bus, "on");
    tour = new Tour<Ev>(steps);
    const detach = tour.attach(bus);
    expect(on.mock.calls.map((c) => c[0]).sort()).toEqual(["cellSelected", "checksRun"]);
    tour.start();
    bus.emit("checksRun");
    expect(tour.step?.id).toBe("cell");
    detach();
    bus.emit("cellSelected", { table: "t", row: 0 });
    expect(tour.step?.id).toBe("cell");
  });

  it("drops events that arrive while it isn't running", () => {
    tour = new Tour<Ev>(steps);
    tour.notify("checksRun");
    tour.start();
    expect(tour.step?.id).toBe("run");
  });

  it("advances on a DOM event only when it passes through the step's target", () => {
    const tip = document.body.appendChild(document.createElement("span"));
    const inner = tip.appendChild(document.createElement("b"));
    const other = document.body.appendChild(document.createElement("span"));
    tour = new Tour<Ev>([{ id: "hover", text: "Hover the icon.", target: () => tip, advanceOn: { dom: "pointerenter" } }, steps[2]]);
    tour.start();
    other.dispatchEvent(new Event("pointerenter"));
    tip.dispatchEvent(new Event("click", { bubbles: true }));
    expect(tour.step?.id).toBe("hover");
    inner.dispatchEvent(new Event("pointerenter"));
    expect(tour.step?.id).toBe("look");
  });

  it("stops pointer input on the overlay from reaching the page's listeners", () => {
    const outside = vi.fn();
    document.addEventListener("click", outside);
    tour = new Tour<Ev>(steps);
    tour.start();
    document.querySelector<HTMLElement>(".tsappkit-tour-dim")!.click();
    button("close")!.click();
    document.removeEventListener("click", outside);
    expect(outside).not.toHaveBeenCalled();
  });

  it("puts the step's text in the panel as text", () => {
    tour = new Tour<Ev>([{ id: "x", title: "Findings", text: "<b>not bold</b>" }]);
    tour.start();
    expect(panel()?.querySelector(".tsappkit-tour-text")?.textContent).toBe("<b>not bold</b>");
    expect(panel()?.querySelector("b")).toBeNull();
    expect(panel()?.querySelector(".tsappkit-tour-count")?.textContent).toBe("1 of 1");
  });

  it("rejects events and payload fields the app's event map doesn't have", () => {
    const typed = () =>
      new Tour<Ev>([
        // @ts-expect-error cellPicked isn't an event of Ev
        { id: "a", text: "", advanceOn: { event: "cellPicked" } },
        // @ts-expect-error cellSelected's payload has no column
        { id: "b", text: "", advanceOn: { event: "cellSelected", where: (d) => d.column === 1 } },
        { id: "c", text: "", offPath: [{ event: "checksRun" }, { dom: "pointerenter" }] },
      ]);
    expect(typed).toBeTypeOf("function");
    // @ts-expect-error cellSelected needs its payload
    const bad = () => tour!.notify("cellSelected", { table: "t" });
    expect(bad).toBeTypeOf("function");
  });
});
