import { defineConfig } from 'tsup';

export default defineConfig([
  {
    entry: [
      'src/index.ts',
      'src/docs/index.ts',
      'src/wasmhost/index.ts',
    ],
    format: ['cjs', 'esm'],
    dts: true,
    splitting: false,
    sourcemap: true,
    clean: true,
    treeshake: true,
    minify: false,
  },
  // The wasm host worker is loaded by URL, not imported, and is a classic script so it can
  // importScripts Go's wasm_exec.js.
  {
    entry: { 'wasmhost/worker': 'src/wasmhost/worker.ts' },
    format: ['iife'],
    outExtension: () => ({ js: '.js' }),
    sourcemap: true,
    clean: false,
    minify: false,
  },
]);
