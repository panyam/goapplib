import { describe, expect, it } from 'vitest';
import { SolidIsland } from './SolidIsland';
import { signalView } from './signalView';
import { useView } from './useView';

interface CountView {
  setCount(n: number): void;
}

// tsappkit's Port in miniature: this package tests against a published tsappkit older than Port.
class Port<V> {
  readonly views = new Set<V>();
  constructor(private replay: (v: V) => void) {}
  attach(v: V) {
    this.views.add(v);
    this.replay(v);
    return () => void this.views.delete(v);
  }
  each(send: (v: V) => void) {
    this.views.forEach(send);
  }
  get size() {
    return this.views.size;
  }
}

describe('useView', () => {
  it('attaches the view while the island is active and detaches it on deactivate', () => {
    let count = 3;
    const port = new Port<CountView>((v) => v.setCount(count));
    const root = document.createElement('div');
    const island = new SolidIsland('counter', root, () => {
      const [n, setCount] = signalView(0);
      useView(port, { setCount });
      return <span>{n()}</span>;
    });

    island.activate();
    expect(root.textContent).toBe('3');
    expect(port.size).toBe(1);

    count = 4;
    port.each((v) => v.setCount(count));
    expect(root.textContent).toBe('4');

    island.deactivate();
    expect(port.size).toBe(0);
  });
});
