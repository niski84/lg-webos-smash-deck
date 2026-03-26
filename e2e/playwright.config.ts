import { defineConfig, devices } from '@playwright/test';

// High port to avoid clashing with dev servers (8088) or other locals.
const port = process.env.LGDECK_E2E_PORT || '18765';
const baseURL = `http://127.0.0.1:${port}`;

export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  workers: 1,
  reporter: 'list',
  use: {
    baseURL,
    trace: 'off',
    screenshot: 'off',
    video: 'off',
    ...devices['Desktop Chrome'],
  },
  webServer: {
    command: `cd .. && PORT=${port} DATA_DIR=./e2e/test-data go run ./cmd/lgdeck`,
    url: baseURL,
    timeout: 120_000,
    reuseExistingServer: !process.env.CI,
    stdout: 'pipe',
    stderr: 'pipe',
  },
});
