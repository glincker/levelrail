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
    // its own chunk, which is what keeps CLAUDE.md 4.12's "the dashboard
    // should not ship the log viewer's bundle" true without hand-written
    // React.lazy() at every route boundary.
    tanstackRouter({
      target: 'react',
      autoCodeSplitting: true,
      routesDirectory: './src/routes',
      generatedRouteTree: './src/routeTree.gen.ts',
    }),
    react(),
    tailwindcss(),
    // Emits web/dist/stats.html on every `npm run build`, a treemap of
    // final chunk sizes. CLAUDE.md 7 requires a bundle size budget in CI;
    // this is the visibility half of that (frontend-plan.md section 2).
    // A per-chunk size-assertion script that fails CI on regression is
    // deferred, see the report for this pass.
    visualizer({
      filename: 'dist/stats.html',
      gzipSize: true,
      brotliSize: true,
    }),
  ],
})
