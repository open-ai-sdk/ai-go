import { defineConfig } from 'vite';

export default defineConfig({
  root: 'browser/app',
  server: {
    proxy: {
      '/chat': 'http://127.0.0.1:8787',
    },
  },
});
