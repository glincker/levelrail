import path from 'node:path'
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { tanstackRouter } from '@tanstack/router-plugin/vite'
import { visualizer } from 'rollup-plugin-visualizer'
import { docsManifestPlugin } from './vite-plugins/docsManifest.js'
import { docsAssetsPlugin } from './vite-plugins/docsAssets.js'
import { precompressPlugin } from './vite-plugins/precompress.js'

// https://vite.dev/config/
export default defineConfig({
  build: {
    // Off by default (including for release.yml's build): the manifest
    // is only useful to scripts/check-bundle-size.js's mermaid-chunk
    // exemption, not to the shipped app, and would otherwise get pulled
    // into the embedded binary by handler.go's `//go:embed all:dist`.
    // CI's web-check job sets BUNDLE_MANIFEST=true right before running
    // that check.
    manifest: process.env.BUNDLE_MANIFEST === 'true',
    rolldownOptions: {
      output: {
        // $initial-tagged groups merge only the first-paint graph; lazy-only
        // modules keep splitting per route. Per-module chunks cost one round trip each.
        codeSplitting: {
          groups: [
            {
              name: 'react',
              test: /node_modules[\\/](react|react-dom|scheduler)[\\/]/,
              priority: 40,
            },
            {
              name: 'tanstack',
              test: /node_modules[\\/]@tanstack[\\/]/,
              tags: ['$initial'],
              priority: 30,
            },
            {
              name: 'icons',
              test: /node_modules[\\/]@phosphor-icons[\\/]/,
              tags: ['$initial'],
              priority: 30,
            },
            {
              name: 'brand-logos',
              test: /node_modules[\\/]@thesvg[\\/]/,
              maxSize: 300_000,
              priority: 25,
            },
            {
              name: 'icons-lazy',
              test: /node_modules[\\/]@phosphor-icons[\\/]/,
              priority: 25,
            },
            {
              name: 'vendor',
              test: /node_modules[\\/]/,
              tags: ['$initial'],
              priority: 20,
            },
            {
              name: 'ui-kit',
              test: /src[\\/]components[\\/]ui[\\/]/,
              priority: 12,
            },
            {
              name: 'data',
              test: /src[\\/](queries|hooks)[\\/]/,
              priority: 11,
            },
            {
              name: 'shell',
              test: /src[\\/]/,
              tags: ['$initial'],
              priority: 10,
            },
          ],
        },
      },
    },
  },
  server: {
    // src/lib/docsContent.ts imports ../../../docs/**/*.md; Vite 8 refuses
    // to serve files outside this list, so the Help page 403s without it.
    fs: { allow: [path.resolve(import.meta.dirname, '..')] },
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
      codeSplittingOptions: {
        defaultBehavior: [
          [
            'component',
            'pendingComponent',
            'errorComponent',
            'notFoundComponent',
          ],
        ],
      },
      routesDirectory: './src/routes',
      generatedRouteTree: './src/routeTree.gen.ts',
    }),
    react(),
    tailwindcss(),
    docsManifestPlugin(),
    docsAssetsPlugin(),
    precompressPlugin(),
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
