// Splits very large bundled doc markdown (generated route tables) into
// several lazily fetched parts so no single chunk nears the bundle budget.
// The loader still resolves to the identical string: parts cut on line
// boundaries and are re-joined in order.
import { readFileSync } from 'node:fs'
import type { Plugin } from 'vite'

const PART_PREFIX = '\0docs-raw-part:'
const MAX_PART_BYTES = Number(process.env.DOCS_RAW_PART_BYTES) || 90_000

function splitLines(text: string, maxBytes: number): string[] {
  const parts: string[] = []
  let current = ''
  for (const line of text.split(/(?<=\n)/)) {
    if (current && Buffer.byteLength(current + line) > maxBytes) {
      parts.push(current)
      current = ''
    }
    current += line
  }
  if (current) parts.push(current)
  return parts
}

export function docsRawSplitPlugin(): Plugin {
  return {
    name: 'docs-raw-split',
    enforce: 'pre',
    resolveId(id) {
      return id.startsWith(PART_PREFIX) ? id : null
    },
    load(id) {
      if (id.startsWith(PART_PREFIX)) {
        const [index, ...rest] = id.slice(PART_PREFIX.length).split(':')
        const file = rest.join(':')
        const parts = splitLines(readFileSync(file, 'utf8'), MAX_PART_BYTES)
        return `export default ${JSON.stringify(parts[Number(index)])}`
      }
      const match = /^(.*[\\/]docs[\\/].+\.md)\?raw$/.exec(id)
      if (!match) return null
      const file = match[1]
      const text = readFileSync(file, 'utf8')
      if (Buffer.byteLength(text) <= MAX_PART_BYTES) return null
      this.addWatchFile(file)
      const count = splitLines(text, MAX_PART_BYTES).length
      const imports = Array.from(
        { length: count },
        (_, i) => `import(${JSON.stringify(`${PART_PREFIX}${i}:${file}`)})`,
      )
      return `export default Promise.all([${imports.join(',')}]).then((m) => m.map((x) => x.default).join(''))`
    },
  }
}
