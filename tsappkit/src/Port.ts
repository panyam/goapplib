/**
 * The views a presenter drives through one typed view interface, its "port"
 * (docs/PRESENTER_CONTRACT_THESIS.md, Principle 3). A presenter keeps one Port
 * per interface it talks to, so one presenter can drive several islands on a
 * page without an EventBus or a store between them:
 *
 *   readonly tools = new Port<ToolsView>((v) => v.setToolState(this.state));
 *   select(t: Tool) { this.state = ...; this.tools.each((v) => v.setToolState(this.state)); }
 *
 * Islands mount at different times (a `visible` one may mount long after the
 * page started), so a view attached late has missed every command sent so far.
 * The replay passed to the constructor runs on each view as it attaches and
 * should send the presenter's current state, which is how a late view catches
 * up. A port takes any number of views, including none: commands to an empty
 * port go nowhere.
 */
export class Port<V> {
  private readonly views = new Set<V>();

  /** @param replay runs on each view as it attaches, to bring it up to date. */
  constructor(private readonly replay?: (view: V) => void) {}

  /**
   * Adds a view, runs the replay on it, and returns the function that removes
   * it again. An island attaches when it activates and detaches when it
   * deactivates (tsappkit-solid's useView does both). Attaching the same view
   * twice keeps one copy, and calling the returned function more than once is
   * harmless.
   */
  attach(view: V): () => void {
    this.views.add(view);
    this.replay?.(view);
    return () => {
      this.views.delete(view);
    };
  }

  /**
   * Sends a command to every view attached when it's called, in the order they
   * attached. A view may detach (or attach another) from inside the command.
   * A view that throws stops the command reaching the views after it.
   */
  each(send: (view: V) => void): void {
    for (const view of [...this.views]) send(view);
  }

  /** How many views are attached. */
  get size(): number {
    return this.views.size;
  }
}
