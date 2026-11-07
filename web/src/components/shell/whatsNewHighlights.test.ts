import { describe, expect, it } from 'vitest'
import { parseBullet, pickHighlights } from './whatsNewHighlights'

describe('parseBullet', () => {
  it('strips markdown and splits the scope', () => {
    const b = parseBullet('**apps:** read-only deploy plan for a repository', 0)
    expect(b.scope).toBe('apps')
    expect(b.text).toBe('Read-only deploy plan for a repository')
    expect(b.group).toBe('apps')
    expect(b.route).toBe('/apps')
  })

  it('flags internal wording', () => {
    expect(
      parseBullet('abilities hook and role-gated controls', 0).internal,
    ).toBe(true)
    expect(parseBullet('refactor the router', 1).internal).toBe(true)
  })

  it('gives no route when nothing matches', () => {
    expect(parseBullet('faster startup', 0).route).toBeNull()
  })
})

describe('pickHighlights', () => {
  const bullets = [
    'add Next.js doctor fixes for build, key, memory and shared cache',
    'abilities hook and role-gated controls for the storage page',
    '**domains:** new readiness steps page for custom domains',
    '**storage:** add volume usage card',
    'tidy copy',
  ]

  it('keeps cards within the cap, drops internal bullets, keeps order', () => {
    const { highlights, rest } = pickHighlights(bullets, 2)
    expect(highlights).toHaveLength(2)
    expect(highlights.every((h) => !h.internal)).toBe(true)
    expect(highlights[0]!.index).toBeLessThan(highlights[1]!.index)
    expect(rest.length).toBe(3)
  })

  it('is deterministic', () => {
    expect(pickHighlights(bullets, 3)).toEqual(pickHighlights(bullets, 3))
  })
})
