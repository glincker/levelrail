import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { engineBrand, engineInitials } from './engineBrand'

// Engines with no thesvg mark must be listed here on purpose.
const EXPLICIT_FALLBACK = new Set<string>()

describe('engineBrand', () => {
  it('maps every managed engine or declares a fallback', () => {
    const yaml = readFileSync(
      resolve(__dirname, '../../../internal/store/database_engines.yaml'),
      'utf8',
    )
    const ids = [...yaml.matchAll(/^\s*-\s*id:\s*(\S+)/gm)].flatMap((m) =>
      m[1] ? [m[1]] : [],
    )
    expect(ids.length).toBeGreaterThan(10)
    for (const id of ids) {
      expect(engineBrand(id) !== null || EXPLICIT_FALLBACK.has(id), id).toBe(
        true,
      )
    }
  })
  it('handles aliases and unknown engines', () => {
    expect(engineBrand('PostgreSQL')).toBe('postgres')
    expect(engineBrand('nope')).toBeNull()
    expect(engineInitials('nope')).toBe('NO')
  })
})
