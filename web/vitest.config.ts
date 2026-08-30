import path from 'node:path'
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'

// Deliberately not vite.config.ts's own tanstackRouter plugin: that
// plugin generates routeTree.gen.ts from src/routes at build time, which
// component tests never need since they exercise components directly,
// not the router.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
  },
})
