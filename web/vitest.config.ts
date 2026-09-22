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
    // Running every test file in its own worker (vitest's default)
    // spins up a fresh jsdom environment per file; under a resource-
    // constrained runner (this sandbox, shared CI runners) that
    // contention alone pushes React Query-driven async assertions like
    // findByText past their default timeout, a real, repeatedly-observed
    // flake (RamFitBadge.test.tsx) that has nothing to do with the
    // component or test logic. Serializing file execution here makes
    // this the actual default for every invocation (CI's plain
    // `npm test`, not just a --no-file-parallelism flag someone has to
    // remember to pass).
    fileParallelism: false,
  },
})
