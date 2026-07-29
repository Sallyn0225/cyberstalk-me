/// <reference types="vitest/config" />
import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: {
    // Mirrors the "@/*" path mapping in tsconfig.app.json.
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  server: {
    // Bind IPv4 loopback explicitly: Vite's default resolves to [::1] only on
    // this Windows setup, which some clients cannot reach.
    host: '127.0.0.1',
    // In dev the Go backend runs separately; proxy the API (including the
    // SSE stream) so the frontend can always use same-origin relative paths.
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  build: {
    // Build straight into the directory the Go server embeds
    // (//go:embed all:web in server/cmd/server/main.go), so `go build`
    // always produces a single binary carrying the current frontend.
    // outDir lives outside the Vite root, so emptyOutDir must be explicit.
    outDir: '../server/cmd/server/web',
    emptyOutDir: true,
  },
  test: {
    // Only pure logic is unit-tested (lib/format, contract parse guards),
    // so the default node environment is enough — no jsdom needed.
    include: ['src/**/*.test.ts'],
  },
})
