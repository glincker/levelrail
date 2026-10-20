import { beforeEach, describe, expect, it } from 'vitest'
import {
  describeQuery,
  rangeSeconds,
  readRecentQueries,
  rememberQuery,
  signatureToRegex,
} from './logExplorer'

describe('signatureToRegex', () => {
  it('matches the lines of a cluster', () => {
    const re = new RegExp(signatureToRegex('db timeout after <num>ms (<ip>)'))
    expect(re.test('db timeout after 301ms (10.0.0.4:5432)')).toBe(true)
    expect(re.test('cache timeout after 301ms (10.0.0.4)')).toBe(false)
  })

  it('escapes regex metacharacters in the literal parts', () => {
    const re = new RegExp(signatureToRegex('cannot open [file.txt] (<num>)'))
    expect(re.test('cannot open [file.txt] (12)')).toBe(true)
    expect(re.test('cannot open xfile.txt] (12)')).toBe(false)
  })
})

describe('recent queries', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })

  it('keeps the newest first, dedupes and caps', () => {
    rememberQuery('web', { q: 'a' })
    rememberQuery('web', { q: 'b' })
    rememberQuery('web', { q: 'a' })
    expect(readRecentQueries('web').map((r) => r.q)).toEqual(['a', 'b'])
    for (let i = 0; i < 12; i++) {
      rememberQuery('web', { q: `q${i}` })
    }
    expect(readRecentQueries('web')).toHaveLength(8)
  })

  it('ignores empty queries and scopes by app', () => {
    rememberQuery('web', {})
    rememberQuery('api', { level: 'error' })
    expect(readRecentQueries('web')).toEqual([])
    expect(readRecentQueries('api')).toHaveLength(1)
  })

  it('describes a query', () => {
    expect(describeQuery({ q: 'boom', level: 'error', field: ['a=1'] })).toBe(
      '"boom" >= error a=1',
    )
  })
})

describe('rangeSeconds', () => {
  it('parses relative ranges', () => {
    expect(rangeSeconds('15m')).toBe(900)
    expect(rangeSeconds('2h')).toBe(7200)
    expect(rangeSeconds('7d')).toBe(604800)
    expect(rangeSeconds('soon')).toBe(0)
  })
})
