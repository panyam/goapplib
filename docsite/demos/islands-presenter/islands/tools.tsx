// The tools island implements ToolsView with a signal, and sends the user's pick up to the
// presenter as an intent.
import type { EventBus, IslandSpec, LCMComponent } from "../../../../tsappkit/src";
import { SolidIsland, signalView, useView } from "../../../../tsappkit-solid/src";
import type { Color, EditorPresenter } from "../presenter";

export default function tools(el: HTMLElement, _island: IslandSpec, ctx: { editor: EditorPresenter }, bus: EventBus): LCMComponent {
  return new SolidIsland(
    "tools",
    el,
    () => {
      const [color, setColor] = signalView<Color>("red");
      useView(ctx.editor.tools, { setColor });
      el.dataset.mounted = "tools";
      return (
        <p>
          {(["red", "green", "blue"] as const).map((c) => (
            <button type="button" id={`pick-${c}`} aria-pressed={color() === c} onClick={() => ctx.editor.pick(c)}>
              {c}
            </button>
          ))}{" "}
          Current: <b id="current">{color()}</b>
        </p>
      );
    },
    bus,
  );
}
