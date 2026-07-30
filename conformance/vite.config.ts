import { defineConfig } from 'vite-plus';

export default defineConfig({
  root: 'browser/app',
  fmt: {
    ignorePatterns: ['src/ui_message_chunk_types.json'],
    singleQuote: true,
  },
  lint: {
    options: {
      typeAware: true,
      typeCheck: true,
    },
  },
  server: {
    proxy: {
      '/chat': 'http://127.0.0.1:8787',
    },
  },
  test: {
    include: ['../../src/**/*.test.ts'],
    testTimeout: 60_000,
  },
});
