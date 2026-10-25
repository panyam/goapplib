import { pad, type Rect } from "./geometry";
import { DEFAULT_LABELS, TourOverlay, type TourLabels } from "./overlay";

/**
 * What a step lets the reader reach: one element, several (a NodeList or an array), a rectangle in
 * viewport coordinates, or a mix. Each becomes a hole in the overlay. Nothing (null, or an empty
 * list) leaves the whole page dimmed, which is how a target that hasn't rendered yet looks until
 * it does: the tour re-reads `target` every frame.
 */
export type TourTarget =
  | Element
  | DOMRectReadOnly
  | Rect
  | Iterable<Element | DOMRectReadOnly | Rect>
  | null
  | undefined;

/**
 * An app event, by its name in the app's event map E, with an optional test of its payload. `where`
 * is typed from the event named, so a payload field the event doesn't carry is a type error.
 */
export type AppTrigger<E> = {
  [K in keyof E & string]: { event: K; where?: (data: E[K]) => boolean };
}[keyof E & string];

/**
 * A DOM event on the step's own target elements, for steps that teach the UI rather than the app
 * (hover for a tooltip, drag a splitter). It matches when an event of this type passes through a
 * target and infers nothing more. It sees nothing on a rectangle target, since a rectangle has no
 * element to pass through.
 */
export interface DomTrigger {
  dom: keyof HTMLElementEventMap;
}

/** Something a step waits for: an app event (`event`) or a DOM event on its target (`dom`). */
export type TourTrigger<E> = AppTrigger<E> | DomTrigger;

/** One step of a tour, written as data. */
export interface TourStep<E> {
  /** Names the step in callbacks and in the overlay's `data-tour-step`. */
  id: string;
  /** Shown above the text, when given. */
  title?: string;
  /** What the reader should do, or what they're looking at. Set as text, never parsed as HTML. */
  text: string;
  /** The regions the reader can reach during the step. Called every frame while the step is live. */
  target?: () => TourTarget;
  /**
   * What finishes the step. Without one the step is something to look at, and the panel shows Next
   * (Done on the last step).
   */
  advanceOn?: TourTrigger<E>;
  /**
   * Events that mean the reader went somewhere else. One that matches `offPath` and not
   * `advanceOn` applies `onOffPath`; naming the same event as `advanceOn` without its `where`
   * catches every other payload of it.
   */
  offPath?: TourTrigger<E> | readonly TourTrigger<E>[];
  /**
   * `"end"` (the default) removes the overlay and leaves the app doing what it does with that
   * input, so the reader has simply left the tour. `"stay"` keeps waiting for `advanceOn`.
   */
  onOffPath?: "end" | "stay";
  /**
   * Does the step's action for the reader, behind a "Show me" button. It should act through the app
   * as a reader would, so the step still advances on the app's event rather than on the button.
   */
  doItForMe?: () => void;
}

/** Why a tour ended: its last step finished, the reader closed it, or the reader went off path. */
export type TourEndReason = "done" | "cancelled" | "offPath";

/** Hooks and settings for a tour. */
export interface TourOptions<E> {
  /** Runs as each step goes live, the first one included. */
  onStep?: (step: TourStep<E>, index: number) => void;
  /** Runs once when the tour ends, with the step it was on. */
  onEnd?: (reason: TourEndReason, step: TourStep<E>, index: number) => void;
  /** How far each hole extends past its target, in px. Defaults to 4. */
  padding?: number;
  /** The panel's button labels. */
  labels?: Partial<TourLabels>;
}

/**
 * The part of an event bus a tour needs: tsappkit's EventBus has it, and so can anything else.
 */
export interface TourEventSource {
  on(type: string, handler: (data?: any) => void): void;
  off(type: string, handler: (data?: any) => void): void;
}

type Seen = { kind: "app"; type: string; data: unknown } | { kind: "dom"; type: string };

const raf: (cb: () => void) => number =
  typeof requestAnimationFrame === "function" ? (cb) => requestAnimationFrame(cb) : (cb) => setTimeout(cb, 16) as unknown as number;
const caf: (id: number) => void =
  typeof cancelAnimationFrame === "function" ? (id) => cancelAnimationFrame(id) : (id) => clearTimeout(id);

/**
 * A guided walk over an app in which the reader does each step. The page is dimmed, the step's
 * targets stay reachable through holes in the dimming, and the step advances when the app reports
 * that the reader did the thing: an app event matched by `advanceOn`, not a click inferred from a
 * selector. E is the app's event map (event name to payload), so a step naming an event the app
 * doesn't have, or a payload field the event doesn't carry, fails to compile:
 *
 *   interface ViewerEvents { checksRun: void; cellSelected: { table: string; row: number } }
 *   const tour = new Tour<ViewerEvents>([
 *     { id: "run", text: "Run the checks.", target: () => runButton, advanceOn: { event: "checksRun" } },
 *     { id: "cell", text: "Click a part in the first row.", target: () => rows,
 *       advanceOn: { event: "cellSelected", where: (d) => d.row === 0 },
 *       offPath: { event: "cellSelected" } },
 *   ]);
 *   tour.attach(bus);   // or call tour.notify(type, data) from the app's own channels
 *   tour.start();
 *
 * An action the app reports no event for can't be a step, and the fix is an app event rather than
 * a cleverer listener: an action nothing outside the app can observe can't be checked by a tour, a
 * browser test or an agent either. The `dom` trigger is the narrow exception, for steps about the
 * UI itself.
 *
 * Esc and the panel's close button end any tour. Input outside the holes (pointer and wheel) is
 * blocked; keyboard input isn't, so a reader can still Tab to the rest of the page.
 */
export class Tour<E extends object = Record<string, any>> {
  private at = -1;
  private overlay: TourOverlay | null = null;
  private frame = 0;
  private domTypes: string[] = [];
  private readonly labels: TourLabels;

  /** @param steps the steps, in order. A tour with no steps never starts. */
  constructor(
    readonly steps: readonly TourStep<E>[],
    private readonly options: TourOptions<E> = {},
  ) {
    this.labels = { ...DEFAULT_LABELS, ...options.labels };
  }

  /** Whether the tour is showing a step. */
  get running(): boolean {
    return this.at >= 0;
  }

  /** The live step's index, or -1 when the tour isn't running. */
  get index(): number {
    return this.at;
  }

  /** The live step, or undefined when the tour isn't running. */
  get step(): TourStep<E> | undefined {
    return this.steps[this.at];
  }

  /**
   * Shows the overlay and step `at` (0 by default). On a running tour it moves to that step. A tour
   * can be started again after it ends.
   */
  start(at = 0): void {
    if (at < 0 || at >= this.steps.length) return;
    if (!this.overlay) {
      this.overlay = new TourOverlay(document, this.labels, {
        close: () => this.end("cancelled"),
        next: () => this.next(),
        showMe: () => this.step?.doItForMe?.(),
      });
      document.addEventListener("keydown", this.onKey, true);
    }
    this.go(at);
    if (!this.frame) this.frame = raf(this.tick);
  }

  /** Moves to the next step, or ends the tour as `"done"` after the last one. */
  next(): void {
    if (!this.running) return;
    if (this.at + 1 >= this.steps.length) this.end("done");
    else this.go(this.at + 1);
  }

  /** Removes the overlay and runs `onEnd`. Does nothing on a tour that isn't running. */
  end(reason: TourEndReason = "cancelled"): void {
    if (!this.running) return;
    const index = this.at;
    this.listenDom([]);
    this.at = -1;
    caf(this.frame);
    this.frame = 0;
    document.removeEventListener("keydown", this.onKey, true);
    this.overlay?.destroy();
    this.overlay = null;
    this.options.onEnd?.(reason, this.steps[index], index);
  }

  /**
   * Tells the tour an app event happened, for an app that keeps its own channels rather than an
   * EventBus. Ignored while the tour isn't running. The live step advances when it matches
   * `advanceOn`, and otherwise applies `onOffPath` when it matches `offPath`. An event whose payload
   * is `void` takes no data.
   */
  notify<K extends keyof E & string>(type: K, ...data: E[K] extends void ? [data?: E[K]] : [data: E[K]]): void {
    this.seen({ kind: "app", type, data: data[0] });
  }

  /**
   * Subscribes to every app event any step names (in `advanceOn` or `offPath`) and passes each to
   * `notify`. The subscriptions last until the returned function is called, across ends and
   * restarts; events arriving while the tour isn't running are ignored.
   */
  attach(source: TourEventSource): () => void {
    const names = new Set<string>();
    for (const s of this.steps) {
      for (const t of [s.advanceOn, ...triggers(s.offPath)]) if (t && "event" in t) names.add(t.event);
    }
    const handlers = [...names].map((type) => {
      const h = (data?: any) => this.seen({ kind: "app", type, data });
      source.on(type, h);
      return [type, h] as const;
    });
    return () => handlers.forEach(([type, h]) => source.off(type, h));
  }

  private go(at: number): void {
    this.at = at;
    const step = this.steps[at];
    this.listenDom(
      [step.advanceOn, ...triggers(step.offPath)].flatMap((t) => (t && "dom" in t ? [t.dom] : [])),
    );
    this.overlay!.show(step.id, {
      index: at,
      count: this.steps.length,
      title: step.title,
      text: step.text,
      next: !step.advanceOn,
      showMe: !!step.doItForMe,
    });
    this.measure();
    this.options.onStep?.(step, at);
  }

  private seen(e: Seen): void {
    const step = this.step;
    if (!step) return;
    if (step.advanceOn && matches(step.advanceOn, e)) this.next();
    else if (triggers(step.offPath).some((t) => matches(t, e)) && (step.onOffPath ?? "end") === "end") {
      this.end("offPath");
    }
  }

  // DOM triggers listen on the document in the capture phase and check the event's path against the
  // step's targets as they are now, so a target that re-renders between frames still counts, and a
  // non-bubbling event (pointerenter) is still seen.
  private listenDom(types: string[]): void {
    for (const t of this.domTypes) document.removeEventListener(t, this.onDom, true);
    this.domTypes = [...new Set(types)];
    for (const t of this.domTypes) document.addEventListener(t, this.onDom, true);
  }

  private readonly onDom = (e: Event): void => {
    const path = e.composedPath();
    if (this.resolve().elements.some((el) => path.includes(el))) this.seen({ kind: "dom", type: e.type });
  };

  private readonly onKey = (e: KeyboardEvent): void => {
    if (e.key !== "Escape") return;
    e.stopPropagation();
    this.end("cancelled");
  };

  private readonly tick = (): void => {
    if (!this.running) return;
    this.measure();
    this.frame = raf(this.tick);
  };

  // Re-measured every frame while a step is live, which covers a resize, a scroll and a layout that
  // moves under the overlay alike; the overlay only touches the DOM when something changed.
  private measure(): void {
    const by = this.options.padding ?? 4;
    this.overlay?.setHoles(this.resolve().rects.map((r) => pad(r, by)));
  }

  private resolve(): { elements: Element[]; rects: Rect[] } {
    const elements: Element[] = [];
    const rects: Rect[] = [];
    const add = (t: Element | DOMRectReadOnly | Rect) => {
      if (isElement(t)) {
        elements.push(t);
        const r = t.getBoundingClientRect();
        if (r.width > 0 && r.height > 0) rects.push({ x: r.x, y: r.y, width: r.width, height: r.height });
      } else {
        rects.push({ x: t.x, y: t.y, width: t.width, height: t.height });
      }
    };
    const target = this.step?.target?.();
    if (target == null) return { elements, rects };
    if (isElement(target) || !isIterable(target)) add(target as Element | Rect);
    else for (const t of target) add(t);
    return { elements, rects };
  }
}

function triggers<E>(t: TourTrigger<E> | readonly TourTrigger<E>[] | undefined): readonly TourTrigger<E>[] {
  if (!t) return [];
  return Array.isArray(t) ? t : [t as TourTrigger<E>];
}

function matches<E>(t: TourTrigger<E>, e: Seen): boolean {
  if ("dom" in t) return e.kind === "dom" && e.type === t.dom;
  const app = t as { event: string; where?: (data: unknown) => boolean };
  return e.kind === "app" && e.type === app.event && (!app.where || app.where(e.data));
}

const isElement = (t: unknown): t is Element => typeof Element !== "undefined" && t instanceof Element;
const isIterable = (t: object): t is Iterable<Element | Rect> => Symbol.iterator in t;
