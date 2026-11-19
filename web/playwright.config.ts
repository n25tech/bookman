import { defineConfig, devices } from '@playwright/test';

// Runs against a running bookman (see README "End-to-end test").
export default defineConfig({
  testDir: './e2e',
  timeout: 60_000,
  retries: 0,
  workers: 1, // the tests share one server, user and book
  use: {
    baseURL: process.env.BOOKMAN_E2E_URL ?? 'http://localhost:8080',
    trace: 'retain-on-failure',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
});
