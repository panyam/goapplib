import { createSignal, type Accessor } from 'solid-js';

/**
 * Create a Solid-backed cell for the "typed view-interface method backed by a
 * signal" pattern: the accessor drives JSX, the returned setter is the
 * command-down method a framework-agnostic presenter calls.
 *
 * The setter always writes via the updater form (`set(() => next)`) rather than
 * `set(next)`. Solid treats a function passed to a signal setter as an updater
 * to run, not a value to store, so a plain `set(next)` would be wrong whenever
 * `S` is itself a function type. Wrapping makes `signalView` a correct
 * "replace the value" setter for any `S`.
 */
export function signalView<S>(initial: S): [Accessor<S>, (next: S) => void] {
  const [value, setValue] = createSignal<S>(initial);
  return [value, (next: S) => setValue(() => next)];
}
