// Writes .br and .gz siblings next to compressible build output so the Go
// server (web/static.go) serves them without compressing per request.
import { brotliCompressSync, constants, gzipSync } from 'node:zlib'
import { readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import type { Plugin } from 'vite'

// Must match compressibleExt in web/static.go.
const COMPRESSIBLE = new Set([
  '.js',
  '.mjs',
  '.css',
  '.html',
  '.svg',
  '.json',
  '.map',
  '.txt',
  '.xml',
  '.webmanifest',
])
const MIN_BYTES = 1024

function walk(dir: string): string[] {
  const out: string[] = []
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name)
    if (entry.isDirectory()) out.push(...walk(full))
    else out.push(full)
  }
  return out
}

export function precompressPlugin(): Plugin {
  let outDir = ''
  return {
    name: 'precompress',
    apply: 'build',
    configResolved(config) {
      outDir = path.resolve(config.root, config.build.outDir)
    },
    closeBundle: {
      order: 'post',
      handler() {
        for (const file of walk(outDir)) {
          if (!COMPRESSIBLE.has(path.extname(file).toLowerCase())) continue
          if (statSync(file).size < MIN_BYTES) continue
          const data = readFileSync(file)
          const br = brotliCompressSync(data, {
            params: {
              [constants.BROTLI_PARAM_QUALITY]: 11,
              [constants.BROTLI_PARAM_SIZE_HINT]: data.length,
            },
          })
          const gz = gzipSync(data, { level: 9 })
          if (br.length < data.length) writeFileSync(`${file}.br`, br)
          if (gz.length < data.length) writeFileSync(`${file}.gz`, gz)
        }
      },
    },
  }
}
