// Serves docs/assets/ (screenshots referenced from markdown via
// resolveDocImageSrc, web/src/lib/docsLinks.ts) at the /docs-assets/ URL
// prefix, both in dev (a server middleware reading straight off disk) and
// in the built app (copied into dist/docs-assets/ at build time, so the
// Go binary's `//go:embed all:dist` picks it up like every other asset).
// Only docs/assets/ is copied, not all of docs/, which is already bundled
// separately via docsManifestPlugin's virtual module.
import {
  createReadStream,
  existsSync,
  readdirSync,
  readFileSync,
  statSync,
} from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import type { Plugin } from 'vite'

const docsAssetsDir = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '../../docs/assets',
)

const URL_PREFIX = '/docs-assets/'

const CONTENT_TYPES: Record<string, string> = {
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.svg': 'image/svg+xml',
  '.webp': 'image/webp',
  '.gif': 'image/gif',
}

function walk(dir: string, base = ''): string[] {
  let out: string[] = []
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const rel = base ? `${base}/${entry.name}` : entry.name
    if (entry.isDirectory()) {
      out = out.concat(walk(path.join(dir, entry.name), rel))
    } else {
      out.push(rel)
    }
  }
  return out
}

export function docsAssetsPlugin(): Plugin {
  return {
    name: 'docs-assets',
    configureServer(server) {
      server.middlewares.use(URL_PREFIX, (req, res, next) => {
        const rel = decodeURIComponent((req.url ?? '').split('?')[0]).replace(
          /^\/+/,
          '',
        )
        const filePath = path.join(docsAssetsDir, rel)
        if (
          !filePath.startsWith(docsAssetsDir) ||
          !existsSync(filePath) ||
          !statSync(filePath).isFile()
        ) {
          next()
          return
        }
        const contentType = CONTENT_TYPES[path.extname(filePath).toLowerCase()]
        if (contentType) res.setHeader('Content-Type', contentType)
        createReadStream(filePath).pipe(res)
      })
    },
    generateBundle() {
      if (!existsSync(docsAssetsDir)) return
      for (const rel of walk(docsAssetsDir)) {
        this.emitFile({
          type: 'asset',
          fileName: `docs-assets/${rel}`,
          source: readFileSync(path.join(docsAssetsDir, rel)),
        })
      }
    },
  }
}
