// What the driver reads: which island modules the browser has evaluated (downloaded) and which
// islands have mounted, in order. Kept apart so loading a module and mounting it are told apart.
export interface ExerciseState {
  loaded: string[];
  mounted: string[];
}

export function state(): ExerciseState {
  const w = window as unknown as { exercise?: ExerciseState };
  w.exercise ??= { loaded: [], mounted: [] };
  return w.exercise;
}
