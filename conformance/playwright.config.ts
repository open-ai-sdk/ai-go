import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './browser',
  fullyParallel: false,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? 'github' : 'list',
  use: {
    baseURL: 'http://127.0.0.1:4173',
    trace: 'retain-on-failure',
  },
  webServer: [
    {
      command:
        'go build -o /tmp/ai-go-conformance-server ./cmd/conformance-server && exec /tmp/ai-go-conformance-server -addr 127.0.0.1:8787',
      port: 8787,
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: 'vite --host 127.0.0.1 --port 4173',
      port: 4173,
      reuseExistingServer: false,
      timeout: 120_000,
    },
  ],
  projects: [
    {
      name: 'chromium',
      use: { browserName: 'chromium' },
    },
  ],
});
