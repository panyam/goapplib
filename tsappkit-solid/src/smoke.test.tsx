import { describe, it, expect } from 'vitest';
import { render } from '@solidjs/testing-library';
import { createSignal } from 'solid-js';

function Badge(props: { label: string }) {
  return <span data-testid="badge">{props.label}</span>;
}

describe('tsappkit-solid toolchain', () => {
  it('compiles and renders a Solid component', () => {
    const { getByTestId } = render(() => <Badge label="ready" />);
    expect(getByTestId('badge').textContent).toBe('ready');
  });

  it('reacts to a signal update', () => {
    const [count, setCount] = createSignal(0);
    const { getByTestId } = render(() => <span data-testid="count">{count()}</span>);
    expect(getByTestId('count').textContent).toBe('0');
    setCount(3);
    expect(getByTestId('count').textContent).toBe('3');
  });
});
