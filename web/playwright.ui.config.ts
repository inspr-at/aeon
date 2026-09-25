// SPDX-License-Identifier: AGPL-3.0-only
import { defineConfig } from '@playwright/test'

const port = process.env.PLAYWRIGHT_PORT ?? '5175'

export default defineConfig({
  testDir: './tests',
  testMatch: '**/*.spec.ts',
  fullyParallel: true,
  workers: 4,
  use: { baseURL: `http://127.0.0.1:${port}`, browserName: 'chromium', reducedMotion: 'reduce' },
  webServer: {
    command: `npm run dev -- --host 127.0.0.1 --port ${port} --strictPort`,
    url: `http://127.0.0.1:${port}`,
    reuseExistingServer: !process.env.CI && !process.env.PLAYWRIGHT_PORT,
  },
})
