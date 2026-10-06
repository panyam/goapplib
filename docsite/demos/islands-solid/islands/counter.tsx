// A SolidIsland: its tree replaces the placeholder Go drew in the slot, in the same task, so the
// slot is never empty in between, and its button updates through a Solid signal.
import { createSignal } from "solid-js";
import type { EventBus, IslandSpec, LCMComponent } from "../../../../tsappkit/src";
import { SolidIsland } from "../../../../tsappkit-solid/src";

export default function counter(el: HTMLElement, _island: IslandSpec, _ctx: unknown, bus: EventBus): LCMComponent {
  return new SolidIsland(
    "counter",
    el,
    () => {
      const [clicks, setClicks] = createSignal(0);
      el.dataset.mounted = "counter";
      return (
        <p>
          A Solid island.{" "}
          <button type="button" onClick={() => setClicks(clicks() + 1)}>
            Clicked {clicks()} {clicks() === 1 ? "time" : "times"}
          </button>
        </p>
      );
    },
    bus,
  );
}
