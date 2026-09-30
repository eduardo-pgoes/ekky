import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// /api is owned by `ekky serve` (ekky-ui.service, port 5178), the same server that serves
// dist/ in production. Dev proxies to it so the API has exactly one implementation.
export default defineConfig({
  plugins: [svelte()],
  server: { port: 5177, strictPort: true, proxy: { '/api': 'http://127.0.0.1:5178' } },
})
