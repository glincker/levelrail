// @vitest-environment node
import { readdirSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import baseline from './esMissingBaseline.json'

const root = join(__dirname, '..', 'locales')

function leafKeys(obj: object, prefix = ''): string[] {
  return Object.entries(obj).flatMap(([k, v]) =>
    typeof v === 'object' && v !== null
      ? leafKeys(v as object, `${prefix}${k}.`)
      : [`${prefix}${k}`],
  )
}

function read(lng: string, file: string): object {
  return JSON.parse(readFileSync(join(root, lng, file), 'utf8')) as object
}

// Only namespaces that already have an es file are held to parity; the rest
// intentionally fall back to English.
function missing(): string[] {
  return readdirSync(join(root, 'es'))
    .filter((f) => f.endsWith('.json'))
    .flatMap((f) => {
      const have = new Set(leafKeys(read('es', f)))
      return leafKeys(read('en', f))
        .filter((k) => !have.has(k))
        .map((k) => `${f.replace('.json', '')}:${k}`)
    })
    .sort()
}

describe('en/es key parity', () => {
  it('has no new English keys missing from es beyond the baseline', () => {
    const known = new Set(baseline.missing)
    expect(missing().filter((k) => !known.has(k))).toEqual([])
  })

  it('keeps the baseline free of keys that are now translated', () => {
    const now = new Set(missing())
    expect(baseline.missing.filter((k) => !now.has(k))).toEqual([])
  })
})
