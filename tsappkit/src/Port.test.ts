import { describe, expect, it } from "vitest";
import { Port } from "./Port";

interface CountView {
  setCount(n: number): void;
}

class Counter {
  count = 0;
  readonly views = new Port<CountView>((v) => v.setCount(this.count));
  bump() {
    this.count++;
    this.views.each((v) => v.setCount(this.count));
  }
}

const recorder = () => {
  const seen: number[] = [];
  return { seen, view: { setCount: (n: number) => seen.push(n) } };
};

describe("Port", () => {
  it("sends each command to every attached view", () => {
    const c = new Counter();
    const a = recorder();
    const b = recorder();
    c.views.attach(a.view);
    c.views.attach(b.view);
    c.bump();
    expect(a.seen).toEqual([0, 1]);
    expect(b.seen).toEqual([0, 1]);
    expect(c.views.size).toBe(2);
  });

  it("brings a view attached late up to the presenter's state", () => {
    const c = new Counter();
    c.bump();
    c.bump();
    const late = recorder();
    c.views.attach(late.view);
    expect(late.seen).toEqual([2]);
  });

  it("stops calling a view once it detaches, and detaching twice is harmless", () => {
    const c = new Counter();
    const a = recorder();
    const b = recorder();
    const detachA = c.views.attach(a.view);
    c.views.attach(b.view);
    detachA();
    detachA();
    c.bump();
    expect(a.seen).toEqual([0]);
    expect(b.seen).toEqual([0, 1]);
    expect(c.views.size).toBe(1);
  });

  it("does nothing with no view attached, and works without a replay", () => {
    const c = new Counter();
    c.bump();
    const bare = new Port<CountView>();
    const a = recorder();
    bare.attach(a.view);
    bare.each((v) => v.setCount(5));
    expect(a.seen).toEqual([5]);
  });

  it("lets a view detach while a command is going out", () => {
    const port = new Port<CountView>();
    const seen: string[] = [];
    let detachFirst = () => {};
    detachFirst = port.attach({ setCount: () => (seen.push("first"), detachFirst()) });
    port.attach({ setCount: () => seen.push("second") });
    port.each((v) => v.setCount(1));
    port.each((v) => v.setCount(2));
    expect(seen).toEqual(["first", "second", "second"]);
  });
});
