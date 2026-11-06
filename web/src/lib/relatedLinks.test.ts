import { existsSync, readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'
import { RELATED_LINKS, relatedLinksFor } from './relatedLinks'

const treePath = join(__dirname, '..', 'routeTree.gen.ts')
// CI test shards run without the generated route tree, the tsc job and local runs have it.
const hasTree = existsSync(treePath)
const tree = hasTree ? readFileSync(treePath, 'utf8') : ''
const ids = new Set(
  [...tree.matchAll(/^\s+id: '([^']+)',?$/gm)].map((m) =>
    m[1] === '/' ? '/' : (m[1] ?? '').replace(/\/$/, ''),
  ),
)
const en = JSON.parse(
  readFileSync(join(__dirname, '..', 'locales/en/relatedLinks.json'), 'utf8'),
) as { links: Record<string, string> }

describe.skipIf(!hasTree)('relatedLinks map', () => {
  it('has at least 13 source pages', () => {
    expect(Object.keys(RELATED_LINKS).length).toBeGreaterThanOrEqual(13)
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
    expect(relatedLinksFor('/domains/$domain/').length).toBeGreaterThan(0)
    expect(relatedLinksFor('/nope')).toEqual([])
  })
})
