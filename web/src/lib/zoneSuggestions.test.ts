import { describe, expect, it } from 'vitest'
import { listedZoneNames } from './zoneSuggestions'

describe('listedZoneNames', () => {
  it('reads zone names from the server message', () => {
    expect(
      listedZoneNames(
        'no zone contains "a.b.com"; the provider lists x.com, y.org and 3 more',
      ),
    ).toEqual(['x.com', 'y.org'])
  })
  it('returns nothing for other messages', () => {
    expect(listedZoneNames('no zone contains "a.b.com"')).toEqual([])
    expect(listedZoneNames(undefined)).toEqual([])
  })
})
