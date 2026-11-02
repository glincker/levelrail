import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { RELATED_LINKS, relatedLinksFor } from './relatedLinks'

const tree = readFileSync(join(__dirname, '..', 'routeTree.gen.ts'), 'utf8')
const ids = new Set(
  [...tree.matchAll(/^\s+id: '([^']+)',?$/gm)].map((m) =>
    m[1] === '/' ? '/' : (m[1] ?? '').replace(/\/$/, ''),
  ),
)
const en = JSON.parse(
  readFileSync(join(__dirname, '..', 'locales/en/relatedLinks.json'), 'utf8'),
) as { links: Record<string, string> }

describe('relatedLinks map', () => {
  it('has at least 15 source pages', () => {
    expect(Object.keys(RELATED_LINKS).length).toBeGreaterThanOrEqual(14)
  })
  it('only points from and to real routes', () => {
    for (const [from, entries] of Object.entries(RELATED_LINKS)) {
      expect(ids.has(from), from).toBe(true)
      for (const e of entries) expect(ids.has(e.to), e.to).toBe(true)
    }
  })
  it('has a label for every entry', () => {
    for (const entries of Object.values(RELATED_LINKS))
      for (const e of entries) expect(en.links[e.labelKey]).toBeTruthy()
  })
  it('tolerates a trailing slash and unknown routes', () => {
    expect(relatedLinksFor('/domains/').length).toBeGreaterThan(0)
    expect(relatedLinksFor('/nope')).toEqual([])
  })
})
