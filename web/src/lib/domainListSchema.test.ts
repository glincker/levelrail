import { describe, expect, it } from 'vitest'
import { analyzeDomainList, createDomainListSchema } from './domainListSchema'

describe('analyzeDomainList', () => {
  it('splits, lowercases, strips scheme and path, and dedupes', () => {
    const r = analyzeDomainList(
      'HTTPS://A.com/path, b.com\nc.com/ *.d.com\nhttp://e.com:80',
    )
    expect(r.domains).toEqual(['a.com', 'b.com', 'c.com', '*.d.com'])
    expect(r.issues).toEqual([{ value: 'e.com:80', reason: 'invalid' }])
  })

  it('flags invalid hostnames', () => {
    const r = analyzeDomainList('nope -bad.com bad-.com a_b.com')
    expect(r.domains).toEqual([])
    expect(r.issues.map((i) => i.reason)).toEqual([
      'invalid',
      'invalid',
      'invalid',
      'invalid',
    ])
  })

  it('rejects hostnames over 253 characters', () => {
    const label = 'a'.repeat(60)
    const long = `${label}.${label}.${label}.${label}.${label}.com`
    expect(long.length).toBeGreaterThan(253 - 1)
    const r = analyzeDomainList(long)
    expect(r.issues).toEqual([{ value: long, reason: 'invalid' }])
  })

  it('flags duplicates after the first occurrence', () => {
    const r = analyzeDomainList('a.com A.com/x a.com')
    expect(r.domains).toEqual(['a.com'])
    expect(r.issues).toEqual([
      { value: 'a.com', reason: 'duplicate' },
      { value: 'a.com', reason: 'duplicate' },
    ])
  })

  it('flags domains already claimed with the owner', () => {
    const r = analyzeDomainList('x.com y.com', new Map([['x.com', 'web']]))
    expect(r.domains).toEqual(['y.com'])
    expect(r.issues).toEqual([
      { value: 'x.com', reason: 'claimed', owner: 'web' },
    ])
  })
})

describe('createDomainListSchema', () => {
  const schema = createDomainListSchema(new Map([['x.com', 'web']]))

  it('accepts a clean list', () => {
    expect(
      schema.safeParse({ app: 'web', domains: 'a.com, b.com' }).success,
    ).toBe(true)
  })

  it('requires an app', () => {
    const r = schema.safeParse({ app: '', domains: 'a.com' })
    expect(r.success ? '' : r.error.issues[0]?.message).toBe('appRequired')
  })

  it('reports empty input and bad lines on the domains path', () => {
    const empty = schema.safeParse({ app: 'web', domains: ' ' })
    expect(
      empty.success ? [] : empty.error.issues.map((i) => i.message),
    ).toEqual(['empty'])
    const bad = schema.safeParse({ app: 'web', domains: 'a.com nope' })
    expect(bad.success ? [] : bad.error.issues.map((i) => i.path[0])).toEqual([
      'domains',
    ])
  })
})
