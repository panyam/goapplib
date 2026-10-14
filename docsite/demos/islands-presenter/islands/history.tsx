// The history island implements HistoryView. It loads "visible", so it usually mounts after some
// picks were made, and the Port's replay hands it those.
import type { EventBus, IslandSpec, LCMComponent } from "../../../../tsappkit/src";
import { SolidIsland, signalView, useView } from "../../../../tsappkit-solid/src";
import type { Color, EditorPresenter } from "../presenter";

export default function history(el: HTMLElement, _island: IslandSpec, ctx: { editor: EditorPresenter }, bus: EventBus): LCMComponent {
  return new SolidIsland(
    "history",
    el,
    () => {
      const [picks, setPicks] = signalView<readonly Color[]>([]);
      useView(ctx.editor.history, { setPicks });
      el.dataset.mounted = "history";
      return (
        <p>
          History: <span id="picks">{picks().join(", ") || "nothing yet"}</span>
        </p>
      );
    },
    bus,
  );
}
