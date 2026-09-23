// SPDX-License-Identifier: AGPL-3.0-only
import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  use: {
    baseURL: process.env.BASE_URL ?? 'http://127.0.0.1:8080',
  },
})
