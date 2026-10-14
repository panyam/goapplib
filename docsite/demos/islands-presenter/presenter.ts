// The demo's presenter: framework-neutral, so it imports no Solid. It owns the state and drives two
// view interfaces, one Port each. Each Port's replay sends the current state to a view as it
// attaches, which is how the history island, mounting only once it's scrolled into view, shows the
// picks made before it existed.
import { Port } from "../../../tsappkit/src";

export type Color = "red" | "green" | "blue";

export interface ToolsView {
  setColor(c: Color): void;
}

export interface HistoryView {
  setPicks(picks: readonly Color[]): void;
}

export class EditorPresenter {
  private color: Color = "red";
  private picks: Color[] = [];

  readonly tools = new Port<ToolsView>((v) => v.setColor(this.color));
  readonly history = new Port<HistoryView>((v) => v.setPicks(this.picks));

  pick(c: Color) {
    this.color = c;
    this.picks = [...this.picks, c];
    this.tools.each((v) => v.setColor(c));
    this.history.each((v) => v.setPicks(this.picks));
  }
}
