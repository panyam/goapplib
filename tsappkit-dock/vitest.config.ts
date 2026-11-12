import { defineConfig } from 'vitest/config';
import solid from 'vite-plugin-solid';

// Solid is only here for dock.solid.test.tsx, which mounts Solid trees in panels. The package
// itself has no Solid code and doesn't depend on it.
export default defineConfig({
  plugins: [solid()],
  test: {
    environment: 'jsdom',
    setupFiles: ['./vitest.setup.ts'],
  },
});
