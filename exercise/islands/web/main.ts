// The exercise page: a real IslandPage over the spec Go wrote. Each registry entry is lazy, so each
// island is its own chunk, loaded when its load strategy says; Go writes modulepreload links for
// the eager ones from esbuild's metafile.
import { BasePage, IslandPage, lazy, type EventBus, type LCMComponent, type Registry } from "../../../tsappkit/src";
import { state } from "./record";

state();

class ExercisePage extends IslandPage<Record<string, never>> {
  protected registry(): Registry<Record<string, never>, HTMLElement, LCMComponent, EventBus> {
    return {
      hero: lazy(() => import("./islands/hero")),
      below: lazy(() => import("./islands/below")),
      narrow: lazy(() => import("./islands/narrow")),
    };
  }
  protected makeContext() {
    return {};
  }
}

BasePage.loadAfterPageLoaded("exercisePage", ExercisePage, "exercise-page");
