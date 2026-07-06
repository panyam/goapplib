import { describe, it, expect } from 'vitest';
import { signalView } from './signalView';

describe('signalView', () => {
  it('updates the accessor when set is called', () => {
    const [value, set] = signalView({ terrain: 0 });
    expect(value()).toEqual({ terrain: 0 });
    set({ terrain: 5 });
    expect(value()).toEqual({ terrain: 5 });
  });

  it('stores a function value instead of treating it as a Solid updater', () => {
    const fn = () => 42;
    const [value, set] = signalView<() => number>(() => 0);
    set(fn);
    expect(value()).toBe(fn);
    expect(value()()).toBe(42);
  });
});
