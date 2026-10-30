import { describe, expect, it } from 'vitest'
import { groupHits, spanIndices } from './paletteGroups'
import {
  buildServerQuery,
  mergeChips,
  parseInput,
  scopeKeyCompletions,
  trailingToken,
} from './paletteQuery'
import { withRecentSearch } from './paletteSearches'
import type { SearchHit } from '../queries/search'

const hit = (kind: SearchHit['kind'], title: string): SearchHit => ({
  ref: `${kind}:${title}`,
  kind,
  title,
  score: 1,
})

describe('parseInput', () => {
  it('turns finished scope tokens into chips and keeps free text', () => {
    expect(parseInput('app:web env:prod restart ')).toEqual({
      chips: [
        { key: 'app', value: 'web' },
        { key: 'env', value: 'prod' },
      ],
      text: 'restart ',
    })
  })

  it('keeps the token still being typed in the text', () => {
    expect(parseInput('app:we')).toEqual({ chips: [], text: 'app:we' })
    expect(parseInput('restart kind:act')).toEqual({
      chips: [],
      text: 'restart kind:act',
    })
  })

  it('ignores unknown scopes', () => {
    expect(parseInput('foo:bar ').chips).toEqual([])
  })

  it('is case insensitive on the key', () => {
    expect(parseInput('KIND:action ').chips).toEqual([
      { key: 'kind', value: 'action' },
    ])
  })
})

describe('buildServerQuery', () => {
  it('joins chips and trimmed text', () => {
    expect(
      buildServerQuery([{ key: 'tag', value: 'x' }], '  restart api '),
    ).toBe('tag:x restart api')
    expect(buildServerQuery([], ' ')).toBe('')
  })
})

describe('mergeChips', () => {
  it('does not duplicate a chip', () => {
    const c = { key: 'app' as const, value: 'web' }
    expect(mergeChips([c], [c, { key: 'env', value: 'prod' }])).toHaveLength(2)
  })
})

describe('trailingToken and scope completion', () => {
  it('reads a scope value being typed', () => {
    expect(trailingToken('go app:fo')).toMatchObject({
      key: 'app',
      value: 'fo',
      start: 3,
    })
  })

  it('reads a partial key and completes it', () => {
    expect(trailingToken('ap')?.partialKey).toBe('ap')
    expect(scopeKeyCompletions('ap')).toEqual(['app'])
    expect(scopeKeyCompletions('k')).toEqual(['kind'])
    expect(scopeKeyCompletions('app')).toEqual([])
  })

  it('has no trailing token after a space', () => {
    expect(trailingToken('app:web ')).toBeNull()
  })
})

describe('groupHits', () => {
  const hits = [
    hit('action', 'Restart api'),
    hit('app', 'api'),
    hit('action', 'Deploy api'),
    ...Array.from({ length: 8 }, (_, i) => hit('domain', `d${i}`)),
  ]

  it('orders groups by their best hit', () => {
    expect(groupHits(hits, new Set()).map((g) => g.kind)).toEqual([
      'action',
      'app',
      'domain',
    ])
  })

  it('caps a group and expands on request', () => {
    const capped = groupHits(hits, new Set()).find((g) => g.kind === 'domain')
    expect(capped?.hits).toHaveLength(4)
    expect(capped?.total).toBe(8)
    const open = groupHits(hits, new Set(['domain'])).find(
      (g) => g.kind === 'domain',
    )
    expect(open?.hits).toHaveLength(8)
  })
})

describe('spanIndices', () => {
  it('expands server spans into code point indices', () => {
    expect(
      [
        ...spanIndices([
          { start: 0, end: 3 },
          { start: 4, end: 6 },
        ]),
      ].sort(),
    ).toEqual([0, 1, 2, 4, 5])
    expect(spanIndices(undefined).size).toBe(0)
  })
})

describe('withRecentSearch', () => {
  it('moves a repeat to the front and caps the list', () => {
    expect(withRecentSearch(['alpha', 'bravo'], 'bravo')).toEqual([
      'bravo',
      'alpha',
    ])
    const full = [
      'one1',
      'two2',
      'three',
      'four4',
      'five5',
      'six66',
      'seven',
      'eight',
    ]
    expect(withRecentSearch(full, 'nine9')).toHaveLength(8)
    expect(withRecentSearch(['alpha'], '  ')).toEqual(['alpha'])
  })
})
