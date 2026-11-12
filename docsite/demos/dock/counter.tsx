import type { Accessor, Setter } from "solid-js";
import { render } from "solid-js/web";

// The Solid panel. Its count lives in a signal outside the panel, so the demo's check can read it.
export function mountCounter(el: HTMLElement, count: Accessor<number>, setCount: Setter<number>): () => void {
  return render(
    () => (
      <>
        <p class="tag">Solid</p>
        <button class="count" onClick={() => setCount(count() + 1)}>
          clicked {count()}
        </button>
      </>
    ),
    el,
  );
}
