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

  it("replaces the slot's server-rendered fallback when it mounts", () => {
    const { root, island } = mount(() => <span data-testid="real">real</span>);
    root.innerHTML = '<div class="skeleton"></div><noscript>The panel needs JavaScript.</noscript>';
    expect(root.querySelector('.skeleton')).not.toBeNull();
    island.activate();
    expect(root.children.length).toBe(1);
    expect(root.querySelector('[data-testid="real"]')?.textContent).toBe('real');
    expect(root.querySelector('.skeleton, noscript')).toBeNull();
  });

  it('keeps its tree on a repeated activate rather than clearing it again', () => {
    const [n, setN] = createSignal(0);
    const { root, island } = mount(() => <span data-testid="n">{n()}</span>);
    island.activate();
    setN(3);
    island.activate();
    expect(root.querySelectorAll('[data-testid="n"]').length).toBe(1);
    setN(4);
    expect(root.querySelector('[data-testid="n"]')?.textContent).toBe('4');
  });
});
