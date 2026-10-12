import { describe, expect, it } from 'vitest'
import {
  cardFields,
  paddingFor,
  priorityClass,
  shouldVirtualize,
  splitColumns,
} from './responsiveTable'

describe('priorityClass', () => {
  it.each([
    [undefined, ''],
    [1, ''],
    [2, 'hidden @2xl/rtable:table-cell'],
    [3, 'hidden @4xl/rtable:table-cell'],
  ] as const)('priority %s', (p, want) => {
    expect(priorityClass(p)).toBe(want)
  })
})

describe('shouldVirtualize', () => {
  it.each([
    [0, undefined, false],
    [50, undefined, false],
    [51, undefined, true],
    [10, 5, true],
    [5, 5, false],
  ] as const)('count %s threshold %s', (n, t, want) => {
    expect(shouldVirtualize(n, t)).toBe(want)
  })
})

describe('splitColumns and cardFields', () => {
  it('uses the first column when none is primary', () => {
    const cols = [{ id: 'a' }, { id: 'b' }]
    expect(splitColumns(cols).primary?.id).toBe('a')
    expect(splitColumns(cols).rest.map((c) => c.id)).toEqual(['b'])
  })
  it('honours primary and hideInCard', () => {
    const cols = [
      { id: 'a' },
      { id: 'b', primary: true },
      { id: 'c', hideInCard: true },
      { id: 'd' },
    ]
    expect(splitColumns(cols).primary?.id).toBe('b')
    expect(cardFields(cols).map((c) => c.id)).toEqual(['a', 'd'])
  })
  it('handles no columns', () => {
    expect(splitColumns([]).primary).toBeUndefined()
  })
})

describe('paddingFor', () => {
  it.each([
    [[], 100, { top: 0, bottom: 0 }],
    [
      [
        { start: 20, end: 40 },
        { start: 40, end: 60 },
      ],
      100,
      { top: 20, bottom: 40 },
    ],
    [[{ start: 0, end: 120 }], 100, { top: 0, bottom: 0 }],
  ] as const)('case %#', (items, total, want) => {
    expect(paddingFor(items, total)).toEqual(want)
  })
})
