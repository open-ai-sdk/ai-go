import { defineConfig } from 'vite-plus';

const apiPort = Number(process.env.AI_GO_CONFORMANCE_API_PORT ?? 8787);

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
      '/chat': `http://127.0.0.1:${apiPort}`,
    },
  },
  test: {
    include: ['../../src/**/*.test.ts'],
    testTimeout: 60_000,
  },
});
