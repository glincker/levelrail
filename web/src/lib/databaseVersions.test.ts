import { describe, expect, it } from 'vitest'
import type { DatabaseVersionInfo } from '../types/databaseVersions'
import {
  badgeFor,
  groupVersions,
  relativeAge,
  selectableVersions,
  sourceLink,
} from './databaseVersions'

const now = new Date('2026-10-14T12:00:00Z')

const versions: DatabaseVersionInfo[] = [
  { version: '18.3', channel: 'stable', recommended: true, eol: '2030-11-14' },
  { version: '17.9', channel: 'stable', recommended: false, eol: '2029-11-08' },
  { version: '16.5', channel: 'lts', recommended: false },
  { version: '15.9', channel: 'stable', recommended: false, eol: '2026-12-01' },
  { version: '13.1', channel: 'stable', recommended: false, eol: '2025-11-13' },
  { version: '19rc1', channel: 'rc', recommended: false },
  { version: '19beta2', channel: 'beta', recommended: false },
]

function at(i: number): DatabaseVersionInfo {
  const v = versions[i]
  if (!v) throw new Error('fixture index')
  return v
}

describe('groupVersions', () => {
  it('orders groups and hides pre-release by default', () => {
    const groups = groupVersions(versions, now, false)
    expect(groups.map((g) => g.id)).toEqual([
      'recommended',
      'stable',
      'lts',
      'eol',
    ])
    expect(groups[1]?.versions.map((v) => v.version)).toEqual(['17.9', '15.9'])
  })

  it('shows pre-release when asked and keeps EOL last and unselectable', () => {
    const groups = groupVersions(versions, now, true)
    expect(groups.map((g) => g.id)).toEqual([
      'recommended',
      'stable',
      'lts',
      'prerelease',
      'eol',
    ])
    expect(selectableVersions(groups)).not.toContain('13.1')
    expect(selectableVersions(groups)).toContain('19rc1')
  })

  it('treats a passed eol date as EOL even when the channel says stable', () => {
    const groups = groupVersions(versions, now, false)
    expect(groups.at(-1)?.versions.map((v) => v.version)).toEqual(['13.1'])
  })
})

describe('badgeFor', () => {
  it('maps channel and dates to a badge', () => {
    expect(badgeFor(at(0), now)).toBe('stable')
    expect(badgeFor(at(2), now)).toBe('lts')
    expect(badgeFor(at(3), now)).toBe('eolSoon')
    expect(badgeFor(at(4), now)).toBe('eol')
    expect(badgeFor(at(5), now)).toBe('rc')
    expect(badgeFor(at(6), now)).toBe('beta')
  })
})

describe('sourceLink and relativeAge', () => {
  it('prefers the lifecycle url and ignores non-https links', () => {
    expect(
      sourceLink({
        eol_url: 'https://endoflife.date/postgresql',
        release_notes_url: 'https://www.postgresql.org/docs/release/',
      }),
    ).toEqual({
      name: 'endoflife.date',
      href: 'https://endoflife.date/postgresql',
    })
    expect(sourceLink({ eol_url: 'javascript:alert(1)' })).toBeNull()
    expect(sourceLink({})).toBeNull()
  })

  it('reports minutes, hours and days', () => {
    expect(relativeAge('2026-10-14T11:58:00Z', now)).toEqual({
      value: -2,
      unit: 'minute',
    })
    expect(relativeAge('2026-10-14T06:00:00Z', now)).toEqual({
      value: -6,
      unit: 'hour',
    })
    expect(relativeAge('2026-10-05T12:00:00Z', now)).toEqual({
      value: -9,
      unit: 'day',
    })
    expect(relativeAge(undefined, now)).toBeNull()
  })
})
