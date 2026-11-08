/// <reference types="vitest/config" />
import { defineConfig, type Plugin } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { writeFileSync } from 'node:fs'

// go:embed needs at least one file in dist/. Vite empties dist/ on every
// build, so we recreate the placeholder afterwards.
const keepFile: Plugin = {
  name: 'dendronotes-keep-dist',
  apply: 'build',
  closeBundle() {
    writeFileSync(new URL('./dist/.keep', import.meta.url), '')
  },
}

const api = 'http://127.0.0.1:8080'

export default defineConfig({
  plugins: [svelte(), keepFile],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // No data: URIs for fonts/images: the CSP only allows fonts from 'self'.
    assetsInlineLimit: 0,
    modulePreload: { polyfill: false },
    // CodeMirror dominates the main chunk (~300 kB gzipped); Vim is split off.
    chunkSizeWarningLimit: 1024,
    sourcemap: false,
  },
  test: {
    environment: 'node',
    include: ['src/**/*.test.ts'],
    // palettes.test.ts reads the design tokens as text (tokens.css?raw).
    css: { include: [/tokens\.css/] },
  },
  server: {
    port: 5173,
    strictPort: true,
    // In dev mode Vite serves the frontend and the Go server the API.
    proxy: {
      '/api': api,
      '/healthz': api,
      '/readyz': api,
    },
  },
})
