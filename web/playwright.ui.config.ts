// SPDX-License-Identifier: AGPL-3.0-only
import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  testMatch: '**/*.spec.ts',
  fullyParallel: true,
  workers: 4,
  use: { baseURL: 'http://127.0.0.1:5175', browserName: 'chromium', reducedMotion: 'reduce' },
  webServer: {
    command: 'npm run dev -- --host 127.0.0.1 --port 5175 --strictPort',
    url: 'http://127.0.0.1:5175',
    reuseExistingServer: !process.env.CI,
  },
})
