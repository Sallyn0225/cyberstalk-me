/// <reference types="vitest/config" />
import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// In dev the agent runs separately on a random loopback port. Start it with
// `agent.exe -setup`, then point the dev server at the address it prints:
//   SETUP_TARGET=http://127.0.0.1:54321 npm run dev
// and open the dev server with the session token in the query string. The
// token normally arrives embedded in the page the agent serves, which the
// Vite dev server does not go through.
const setupTarget = process.env.SETUP_TARGET ?? 'http://127.0.0.1:8099'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    // Mirrors the "@/*" path mapping in tsconfig.app.json.
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  server: {
    // Bind IPv4 loopback explicitly, matching web/: Vite's default resolves to
    // [::1] only on this Windows setup, which some clients cannot reach.
    host: '127.0.0.1',
    proxy: {
      '/api': {
        target: setupTarget,
        changeOrigin: true,
      },
    },
  },
  build: {
    // Build straight into the directory the agent embeds (//go:embed all:webui
    // in cmd/agent/webui.go), so `go build` always produces a single exe
    // carrying the current UI. outDir lives outside the Vite root, so
    // emptyOutDir must be explicit.
    outDir: '../cmd/agent/webui',
    emptyOutDir: true,
  },
  test: {
    // Only pure logic is unit-tested (paste parsing, rule reordering, the
    // confirmation phrase), so the default node environment is enough.
    include: ['src/**/*.test.ts'],
  },
})
