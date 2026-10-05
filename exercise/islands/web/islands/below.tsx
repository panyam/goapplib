// The below island as a real SolidIsland (goapplib issue 67), so the exercise runs tsappkit-solid
// in a browser: its slot's fallback has to go when it mounts (issue 39), and its button has to
// update through a Solid signal.
import { createSignal } from "solid-js";
import type { EventBus, IslandSpec, LCMComponent } from "../../../../tsappkit/src";
import { SolidIsland } from "../../../../tsappkit-solid/src";
import { state } from "../record";

state().loaded.push("below");

export default function below(el: HTMLElement, _island: IslandSpec, _ctx: unknown, bus: EventBus): LCMComponent {
  return new SolidIsland("below", el, () => {
    const [clicks, setClicks] = createSignal(0);
    el.dataset.mountedMs = String(Math.round(performance.now()));
    el.dataset.mounted = "below";
    state().mounted.push("below");
    return (
      <p>
        below mounted (Solid){" "}
        <button type="button" data-testid="below-clicks" onClick={() => setClicks(clicks() + 1)}>
          clicked {clicks()} times
        </button>
      </p>
    );
  }, bus);
}
