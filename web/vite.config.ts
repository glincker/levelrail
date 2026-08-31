import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { tanstackRouter } from '@tanstack/router-plugin/vite'
import { visualizer } from 'rollup-plugin-visualizer'

// https://vite.dev/config/
export default defineConfig({
  server: {
    // Dev-server only: `vite build`'s output never reads this block, so
    // there's no risk of a hardcoded localhost target leaking into the
    // embedded production frontend. Without this, `npm run dev` has no
    // way to reach the Go backend's API at all (it serves only the
    // frontend on its own port), which is also a prerequisite for
    // APP_DEV_MODE to matter in practice: see
    // internal/api/devmode.go's doc comment for the auth-bypass side of
    // this pairing.
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
    },
  },
  resolve: {
    alias: {
      // shadcn/ui's import alias convention, matching tsconfig.app.json's
      // "@/*" path mapping so editor and bundler resolution agree.
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  plugins: [
    // Must come before @vitejs/plugin-react per TanStack Router docs: it
    // generates routeTree.gen.ts and (with autoCodeSplitting) splits each
    // route file's component/loader/pendingComponent/errorComponent into
    // its own chunk, which is what keeps the dashboard from shipping the
    // log viewer's bundle without hand-written React.lazy() at every
    // route boundary.
    tanstackRouter({
      target: 'react',
      autoCodeSplitting: true,
      routesDirectory: './src/routes',
      generatedRouteTree: './src/routeTree.gen.ts',
    }),
    react(),
    tailwindcss(),
    // Emits web/dist/stats.html, a treemap of final chunk sizes. Gated
    // behind ANALYZE so it doesn't run on every plain `npm run build`, only
    // an explicit `ANALYZE=true npm run build`. The actual budget
    // enforcement is scripts/check-bundle-size.js, run in CI's web-check
    // job; this plugin is just the manual-inspection visibility half.
    process.env.ANALYZE
      ? visualizer({
          filename: 'dist/stats.html',
          gzipSize: true,
          brotliSize: true,
        })
      : null,
  ],
})
