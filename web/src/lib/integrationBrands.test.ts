import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'
import { BRAND_ICON_NAMES } from '../components/BrandIcon'
import { INTEGRATION_GLYPHS, integrationGlyph } from './integrationBrands'

// Catalogue ids live in the Go registry; parse them so a new provider cannot ship without a glyph.
const catalogSrc = readFileSync(
  resolve(__dirname, '../../../internal/connhub/catalog.go'),
  'utf8',
)

function catalogIds(): Array<[string, string]> {
  const kinds: Record<string, string> = {
    KindGit: 'git',
    KindRegistry: 'registry',
    KindNotify: 'notify',
    KindSecret: 'secret',
    KindDNS: 'dns',
    KindCloud: 'cloud',
    KindWebhook: 'webhook',
  }
  const out: Array<[string, string]> = []
  for (const seg of catalogSrc.split('entries(').slice(1)) {
    const kind = kinds[/^(Kind\w+)/.exec(seg)?.[1] ?? '']
    if (!kind) continue
    for (const p of seg.matchAll(/"([a-z0-9_]+)", "[^"]+"/g)) {
      out.push([kind, p[1] ?? ''])
    }
  }
  return out
}

describe('integrationBrands', () => {
  it('covers every catalogue id from the backend', () => {
    const ids = catalogIds()
    expect(ids.length).toBeGreaterThan(40)
    for (const [kind, id] of ids) {
      expect(integrationGlyph(kind, id), `${kind}:${id}`).not.toBeNull()
    }
  })

  it('only references brand marks the app can render', () => {
    const known = new Set<string>(BRAND_ICON_NAMES)
    for (const entries of Object.values(INTEGRATION_GLYPHS)) {
      for (const [id, spec] of Object.entries(entries)) {
        if (spec.brand) expect(known.has(spec.brand), id).toBe(true)
      }
    }
  })

  it('falls back to null for unknown providers', () => {
    expect(integrationGlyph('notify', 'nope')).toBeNull()
  })
})
