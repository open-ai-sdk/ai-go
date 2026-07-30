import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    include: ['src/**/*.test.ts'],
    // The harness reads fixture files from disk and drives web streams; no browser
    // environment is needed and node keeps failures readable.
    environment: 'node',
  },
});
