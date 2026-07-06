import { describe, it, expect, vi } from 'vitest';
import { createSignal, onCleanup, type JSX } from 'solid-js';
import { SolidIsland } from './SolidIsland';

function mount(renderRoot: () => JSX.Element) {
  const root = document.createElement('div');
  document.body.appendChild(root);
  const island = new SolidIsland('test-island', root, renderRoot);
  return { root, island };
}

describe('SolidIsland', () => {
  it('mounts the Solid tree on activate', () => {
    const { root, island } = mount(() => <span data-testid="hi">hello</span>);
    expect(root.textContent).toBe('');
    island.activate();
    expect(root.querySelector('[data-testid="hi"]')?.textContent).toBe('hello');
  });

  it('re-renders on signal update while mounted', () => {
    const [count, setCount] = createSignal(0);
    const { root, island } = mount(() => <span data-testid="n">{count()}</span>);
    island.activate();
    expect(root.querySelector('[data-testid="n"]')?.textContent).toBe('0');
    setCount(2);
    expect(root.querySelector('[data-testid="n"]')?.textContent).toBe('2');
  });

  it('disposes on deactivate: runs onCleanup and removes nodes', () => {
    const cleanup = vi.fn();
    const { root, island } = mount(() => {
      onCleanup(cleanup);
      return <span data-testid="x">x</span>;
    });
    island.activate();
    expect(root.querySelector('[data-testid="x"]')).not.toBeNull();
    island.deactivate();
    expect(cleanup).toHaveBeenCalledTimes(1);
    expect(root.querySelector('[data-testid="x"]')).toBeNull();
  });

  it('is idempotent on repeated activate', () => {
    const { root, island } = mount(() => <span data-testid="one">one</span>);
    island.activate();
    island.activate();
    expect(root.querySelectorAll('[data-testid="one"]').length).toBe(1);
  });
});
