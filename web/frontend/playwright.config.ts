import path from 'node:path';
import { defineConfig, devices } from '@playwright/test';
import dotenv from 'dotenv';

dotenv.config({ path: path.resolve(__dirname, '.env'), quiet: true });

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 1 : undefined,
  reporter: process.env.CI
    ? [['list'], ['html', { open: 'never' }], ['github']]
    : [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL: process.env.E2E_BASE_URL ?? 'http://127.0.0.1:5173',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
      testIgnore: '**/heavy/**',
    },
    {
      name: 'heavy',
      use: { ...devices['Desktop Chrome'] },
      testMatch: '**/heavy/**/*.spec.ts',
      testIgnore: '**/heavy/**/*.steam.spec.ts',
      timeout: 10 * 60_000,
    },
    {
      // Real steamcmd installs (HLDS app 90) are slow and flaky — nightly only.
      name: 'heavy-steam',
      use: { ...devices['Desktop Chrome'] },
      testMatch: '**/heavy/**/*.steam.spec.ts',
      timeout: 25 * 60_000,
    },
  ],
  webServer: {
    command: 'npm run dev -- --host 127.0.0.1 --port 5173 --strictPort',
    url: 'http://127.0.0.1:5173',
    timeout: 120_000,
    reuseExistingServer: !process.env.CI,
    stdout: 'pipe',
    stderr: 'pipe',
  },
});
