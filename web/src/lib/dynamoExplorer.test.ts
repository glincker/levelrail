import { describe, expect, it } from 'vitest'
import {
  cellText,
  gridColumns,
  plainAttr,
  plainMap,
  typedKey,
} from './dynamoExplorer'

describe('dynamoExplorer', () => {
  it('unwraps typed attributes, nested maps and lists', () => {
    expect(
      plainMap({
        id: { S: 'a1' },
        n: { N: '4.5' },
        ok: { BOOL: true },
        tags: { SS: ['x', 'y'] },
        meta: { M: { deep: { L: [{ N: '1' }, { NULL: true }] } } },
      }),
    ).toEqual({
      id: 'a1',
      n: 4.5,
      ok: true,
      tags: ['x', 'y'],
      meta: { deep: [1, null] },
    })
    expect(plainAttr({ B: 'AAE=' })).toBe('b64:AAE=')
  })

  it('shows containers as JSON in cells and blanks for a missing attribute', () => {
    expect(cellText({ M: { a: { S: 'b' } } })).toBe('{"a":"b"}')
    expect(cellText(undefined)).toBe('')
  })

  it('puts key columns first and keeps first-seen order', () => {
    expect(
      gridColumns(
        [{ b: { S: '1' } }, { a: { S: '2' }, id: { S: '3' } }],
        ['id'],
      ),
    ).toEqual(['id', 'b', 'a'])
  })

  it('builds typed keys and rejects non numeric numbers', () => {
    expect(typedKey('S', 'abc')).toEqual({ S: 'abc' })
    expect(typedKey('N', '12')).toEqual({ N: '12' })
    expect(typedKey('N', '1x')).toBeNull()
    expect(typedKey('S', '')).toBeNull()
  })
})
