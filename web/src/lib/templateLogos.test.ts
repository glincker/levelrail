/// <reference types="node" />
import { readdirSync, readFileSync } from 'node:fs'
import path from 'node:path'
import { describe, expect, it } from 'vitest'
import { monogramTone } from './templateTint'
import {
  COMPOSITE_LOGOS,
  KNOWN_MISSING_LOGOS,
  TEMPLATE_LOGO_LOADERS,
} from './templateLogos'

const CATALOG_DIR = path.resolve(
  import.meta.dirname,
  '../../../internal/catalog',
)

function catalogIds(): string[] {
  const ids = new Set<string>()
  for (const file of readdirSync(CATALOG_DIR)) {
    if (!file.endsWith('.go') || file.endsWith('_test.go')) continue
    const src = readFileSync(path.join(CATALOG_DIR, file), 'utf8')
    for (const m of src.matchAll(/^\s+ID:\s+"([a-z0-9-]+)"/gm)) {
      ids.add(m[1] as string)
    }
  }
  for (const dir of readdirSync(path.join(CATALOG_DIR, 'manifests'))) {
    ids.add(dir)
  }
  const contribDir = path.join(CATALOG_DIR, 'contrib')
  for (const file of readdirSync(contribDir)) {
    const src = readFileSync(path.join(contribDir, file), 'utf8')
    const m = /^id:\s*"?([a-z0-9-]+)"?/m.exec(src)
    if (m) ids.add(m[1] as string)
  }
  return [...ids].sort()
}

describe('template logo coverage', () => {
  const ids = catalogIds()

  it('reads a plausible catalog', () => {
    expect(ids.length).toBeGreaterThan(300)
  })

  it('gives every catalog template a mark or a known-missing entry', () => {
    const uncovered = ids.filter(
      (id) =>
        !(id in TEMPLATE_LOGO_LOADERS) &&
        !(id in COMPOSITE_LOGOS) &&
        !KNOWN_MISSING_LOGOS.includes(id),
    )
    expect(uncovered).toEqual([])
  })

  it('keeps the known-missing list honest', () => {
    const stale = KNOWN_MISSING_LOGOS.filter(
      (id) => id in TEMPLATE_LOGO_LOADERS || id in COMPOSITE_LOGOS,
    )
    expect(stale).toEqual([])
    expect(KNOWN_MISSING_LOGOS.filter((id) => !ids.includes(id))).toEqual([])
  })
})

describe('monogramTone', () => {
  it('is deterministic per id', () => {
    expect(monogramTone('kutt')).toBe(monogramTone('kutt'))
  })

  it('spreads ids across more than one tone', () => {
    const tones = new Set(KNOWN_MISSING_LOGOS.map((id) => monogramTone(id)))
    expect(tones.size).toBeGreaterThan(1)
  })
})
