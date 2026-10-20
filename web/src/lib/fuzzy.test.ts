import { describe, expect, it } from 'vitest'
import { fuzzyFilter, fuzzyScore } from './fuzzy'

describe('fuzzyScore', () => {
  const cases: Array<[string, string, boolean]> = [
    ['', 'anything', true],
    ['web', 'web', true],
    ['wb', 'web', true],
    ['bw', 'web', false],
    ['xyz', 'web', false],
    ['RESTART', 'Restart web', true],
    ['rw', 'Restart web', true],
    ['zz', 'Restart web', false],
  ]
  it.each(cases)('query %j on %j matches=%s', (query, text, matches) => {
    expect(fuzzyScore(query, text) !== null).toBe(matches)
  })

  it('ranks prefix above mid-word and subsequence matches', () => {
    const prefix = fuzzyScore('app', 'Apps') ?? -Infinity
    const mid = fuzzyScore('app', 'Snapp') ?? -Infinity
    const subseq = fuzzyScore('aps', 'Apps') ?? -Infinity
    expect(prefix).toBeGreaterThan(mid)
    expect(mid).toBeGreaterThan(subseq)
  })
})

describe('fuzzyFilter', () => {
  const labels = ['Settings', 'Status', 'Nodes', 'Apps']
  it('keeps order for an empty query', () => {
    expect(fuzzyFilter(labels, '  ', (s) => s)).toEqual(labels)
  })
  it('drops non-matches and sorts best first', () => {
    expect(fuzzyFilter(labels, 'st', (s) => s)).toEqual(['Status', 'Settings'])
  })
  it('returns empty when nothing matches', () => {
    expect(fuzzyFilter(labels, 'zzz', (s) => s)).toEqual([])
  })

  describe('keywords', () => {
    const domains = [
      { domain: 'blog.example.com', app: 'web' },
      { domain: 'web.example.net', app: 'site' },
      { domain: 'api.example.com', app: 'api-gateway' },
    ]
    const find = (q: string) =>
      fuzzyFilter(
        domains,
        q,
        (d) => d.domain,
        (d) => d.app,
      ).map((d) => d.domain)

    it('matches on keywords when the label does not match', () => {
      expect(find('gateway')).toEqual(['api.example.com'])
    })

    it('ranks a label hit above a keyword-only hit', () => {
      expect(find('web')).toEqual(['web.example.net', 'blog.example.com'])
    })

    it('does not change results without a keywords getter', () => {
      expect(fuzzyFilter(domains, 'gateway', (d) => d.domain)).toEqual([])
    })
  })
})
