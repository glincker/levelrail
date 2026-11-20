import { readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'
import { splitLines } from './docsRawSplit.js'

const PART_BYTES = 90_000
const MAX_LINE_BYTES = 4_000
const docsDir = path.resolve(import.meta.dirname, '../../docs')

describe.each(['api-reference.md', 'cli-reference.md'])(
  'docs raw split: %s',
  (name) => {
    const original = readFileSync(path.join(docsDir, name), 'utf8')
    const parts = splitLines(original, PART_BYTES)

    it('joins back to the original byte for byte', () => {
      const joined = parts.join('')
      expect(joined).toBe(original)
      expect(
        Buffer.from(joined, 'utf8').equals(Buffer.from(original, 'utf8')),
      ).toBe(true)
    })

    it('splits into 3 parts, each at most the part size plus one line, cut on line ends', () => {
      expect(parts).toHaveLength(3)
      for (const [i, part] of parts.entries()) {
        expect(Buffer.byteLength(part)).toBeLessThanOrEqual(
          PART_BYTES + MAX_LINE_BYTES,
        )
        if (i < parts.length - 1) expect(part.endsWith('\n')).toBe(true)
      }
    })

    it('keeps the final ## heading from the last part exactly once in the joined text', () => {
      const headings = original.split('\n').filter((l) => l.startsWith('## '))
      const last = headings[headings.length - 1]
      const lines = parts.join('').split('\n')
      expect(lines.filter((l) => l === last)).toHaveLength(1)
      expect(parts[parts.length - 1].split('\n')).toContain(last)
    })
  },
)
