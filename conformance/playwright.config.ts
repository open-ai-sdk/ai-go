import { defineConfig } from '@playwright/test';

const apiPort = Number(process.env.AI_GO_CONFORMANCE_API_PORT ?? 8787);

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
      command: `go build -o /tmp/ai-go-conformance-server ../examples/chat-server && exec /tmp/ai-go-conformance-server -addr 127.0.0.1:${apiPort}`,
      port: apiPort,
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: 'vp dev --host 127.0.0.1 --port 4173',
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
