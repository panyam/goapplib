// When each island's code arrived and when it mounted, in ms since the page started, for the
// demo's table and its checks. An island module records "loaded" when it's evaluated, which for a
// lazy island is when its chunk arrives.
export interface Times {
  loaded?: number;
  mounted?: number;
}
const times: Record<string, Times> = {};
const listeners: (() => void)[] = [];

export function record(name: string, what: keyof Times) {
  (times[name] ??= {})[what] = Math.round(performance.now());
  for (const l of listeners) l();
}

export function timesOf(name: string): Times {
  return times[name] ?? {};
}

export function onChange(l: () => void) {
  listeners.push(l);
}
