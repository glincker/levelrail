import { describe, expect, it } from 'vitest'
import { filterGlossary } from './glossary'

const entries = [
  { key: 'b', label: 'Replicas', body: 'Copies of your app.' },
  { key: 'a', label: 'Agent', body: 'Runs containers on a node.' },
]

describe('filterGlossary', () => {
  it('sorts A to Z and filters by label or body', () => {
    expect(filterGlossary(entries, '').map((e) => e.key)).toEqual(['a', 'b'])
    expect(filterGlossary(entries, 'COPIES').map((e) => e.key)).toEqual(['b'])
    expect(filterGlossary(entries, 'zzz')).toEqual([])
  })
})
