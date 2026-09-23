// SPDX-License-Identifier: AGPL-3.0-only
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  // Never inline assets as data: URLs; the server CSP (default-src 'self') blocks them.
  build: { assetsInlineLimit: 0 },
  server: { proxy: { '/api': 'http://127.0.0.1:8080' } },
})
