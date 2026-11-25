import { describe, expect, it } from 'vitest'
import {
  checkMongoJson,
  documentTitle,
  ejsonScalar,
  formatIndexKey,
} from './mongoExplorer'

describe('checkMongoJson', () => {
  it('treats empty text as unset', () => {
    expect(checkMongoJson('  ', 'filter')).toEqual({
      ok: true,
      value: undefined,
    })
  })
  it('accepts an object filter', () => {
    expect(checkMongoJson('{"a": {"$gt": 1}}', 'filter')).toEqual({
      ok: true,
      value: { a: { $gt: 1 } },
    })
  })
  it('flags syntax errors and non-objects', () => {
    expect(checkMongoJson('{a:1}', 'filter')).toEqual({
      ok: false,
      error: 'syntax',
    })
    expect(checkMongoJson('[1]', 'filter')).toEqual({
      ok: false,
      error: 'notObject',
    })
  })
  it('requires sort directions of 1 or -1', () => {
    expect(checkMongoJson('{"a": -1}', 'sort').ok).toBe(true)
    expect(checkMongoJson('{"a": 2}', 'sort')).toEqual({
      ok: false,
      error: 'sortDirection',
    })
  })
  it('rejects string projection values', () => {
    expect(checkMongoJson('{"a": 1}', 'projection').ok).toBe(true)
    expect(checkMongoJson('{"a": "x"}', 'projection')).toEqual({
      ok: false,
      error: 'projectionValue',
    })
  })
})

describe('extended JSON helpers', () => {
  it('collapses single key wrappers', () => {
    expect(ejsonScalar({ $oid: 'abc' })).toEqual({
      label: 'ObjectId',
      text: 'abc',
    })
    expect(ejsonScalar({ $date: { $numberLong: '0' } })).toEqual({
      label: 'Date',
      text: '1970-01-01T00:00:00.000Z',
    })
    expect(ejsonScalar({ a: 1 })).toBeNull()
  })
  it('titles a document by its id', () => {
    expect(documentTitle({ _id: { $oid: 'abc' } })).toBe('abc')
    expect(documentTitle({ name: 'x' })).toBe('')
  })
  it('formats an index key', () => {
    expect(formatIndexKey({ a: 1, b: -1 })).toBe('a: 1, b: -1')
  })
})
