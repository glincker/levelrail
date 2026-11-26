import { describe, expect, it } from 'vitest'
import { toCsv, toJson } from './export'

describe('toCsv', () => {
  it('quotes commas, quotes and newlines and writes null as empty', () => {
    const csv = toCsv(
      ['a', 'b'],
      [
        ['x,y', 'say "hi"'],
        ['line\nbreak', null],
      ],
    )
    expect(csv).toBe('a,b\r\n"x,y","say ""hi"""\r\n"line\nbreak",\r\n')
  })

  it('prefixes formula starters with an apostrophe but keeps plain numbers', () => {
    const csv = toCsv(
      ['v'],
      [['=SUM(A1)'], ['+cmd'], ['-2+3'], ['@x'], ['-5'], ['1.5e3']],
    )
    expect(csv.split('\r\n').slice(1, 7)).toEqual([
      "'=SUM(A1)",
      "'+cmd",
      "'-2+3",
      "'@x",
      '-5',
      '1.5e3',
    ])
  })

  it('neutralizes headers and pads short rows', () => {
    expect(toCsv(['=h', 'b'], [['1']])).toBe("'=h,b\r\n1,\r\n")
  })
})

describe('toJson', () => {
  it('maps rows to objects, keeps null and stringifies bigint', () => {
    const out = JSON.parse(
      toJson(['id', 'n'], [[9007199254740993n, null]]),
    ) as unknown
    expect(out).toEqual([{ id: '9007199254740993', n: null }])
  })

  it('suffixes duplicate column names', () => {
    const out = JSON.parse(toJson(['a', 'a'], [['1', '2']])) as unknown
    expect(out).toEqual([{ a: '1', a_2: '2' }])
  })
})
