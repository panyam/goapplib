// The exercise page: a real IslandPage over the spec Go wrote. The registry is today's: each entry
// is a factory imported up front, so every island's module loads with the page. Goapplib issue 35
// makes entries lazy and issue 36 makes the page honour each island's load strategy; the driver's
// pending checks flip as they land.
import { BasePage, IslandPage, type EventBus, type LCMComponent, type Registry } from "../../../tsappkit/src";
import { below } from "./islands/below";
import { hero } from "./islands/hero";
import { narrow } from "./islands/narrow";
import { state } from "./record";

state();

class ExercisePage extends IslandPage<Record<string, never>> {
  protected registry(): Registry<Record<string, never>, HTMLElement, LCMComponent, EventBus> {
    return { hero, below, narrow };
  }
  protected makeContext() {
    return {};
  }
}

BasePage.loadAfterPageLoaded("exercisePage", ExercisePage, "exercise-page");
