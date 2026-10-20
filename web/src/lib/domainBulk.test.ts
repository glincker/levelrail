import { describe, expect, it } from 'vitest'
import {
  addDomainsIndependently,
  parseDomainInput,
  splitDomainInput,
} from './domainBulk'

describe('splitDomainInput', () => {
  it('splits newlines, commas, semicolons and spaces', () => {
    expect(splitDomainInput('a.com, b.com\nc.com;d.com  e.com')).toEqual([
      'a.com',
      'b.com',
      'c.com',
      'd.com',
      'e.com',
    ])
  })

  it('lowercases and strips scheme, path and trailing dot', () => {
    expect(splitDomainInput('HTTPS://App.Example.com/path\nfoo.com.')).toEqual([
      'app.example.com',
      'foo.com',
    ])
  })
})

describe('parseDomainInput', () => {
  it('de-duplicates and reports each category', () => {
    const claimed = new Map([['taken.com', 'other']])
    const parsed = parseDomainInput(
      'a.com, A.com, b.com, nope, taken.com, *.wild.com',
      claimed,
    )
    expect(parsed.add).toEqual(['a.com', 'b.com', '*.wild.com'])
    expect(parsed.duplicates).toEqual(['a.com'])
    expect(parsed.invalid).toEqual(['nope'])
    expect(parsed.claimed).toEqual([{ domain: 'taken.com', app: 'other' }])
  })

  it('returns nothing for blank input', () => {
    expect(parseDomainInput('  \n ').add).toEqual([])
  })
})

describe('addDomainsIndependently', () => {
  it('keeps going after a failure and reports each outcome', async () => {
    const seen: string[] = []
    const outcomes = await addDomainsIndependently(
      ['a.com', 'bad.com', 'c.com'],
      (d) => {
        seen.push(d)
        return d === 'bad.com'
          ? Promise.reject(new Error('already in use'))
          : Promise.resolve()
      },
    )
    expect(seen).toEqual(['a.com', 'bad.com', 'c.com'])
    expect(outcomes).toEqual([
      { domain: 'a.com', ok: true },
      { domain: 'bad.com', ok: false, error: 'already in use' },
      { domain: 'c.com', ok: true },
    ])
  })
})
