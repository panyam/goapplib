import { onCleanup } from 'solid-js';

/**
 * Attaches `view` to a presenter's port for as long as the current Solid owner
 * lives. Called from a SolidIsland's render function, that's from the island's
 * activate to its deactivate, so a presenter never calls into an island that
 * has gone. The port is typed structurally so this works with tsappkit's Port
 * or anything with the same attach.
 */
export function useView<V>(port: { attach(view: V): () => void }, view: V): void {
  onCleanup(port.attach(view));
}
