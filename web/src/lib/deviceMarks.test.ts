import { describe, expect, it } from 'vitest'
import { deviceMarks, lastSeenTone } from './deviceMarks'

describe('deviceMarks', () => {
  it.each([
    ['Chrome on macOS', 'chrome', 'apple', 'browser'],
    ['Firefox on Windows', 'firefox', 'windows', 'browser'],
    ['Safari on iOS', 'safari', 'apple', 'browser'],
    ['Edge on Windows', 'edge', 'windows', 'browser'],
    ['Command line client', null, null, 'cli'],
    ['Unknown browser', null, null, 'unknown'],
  ])('%s', (label, browser, os, kind) => {
    expect(deviceMarks(label)).toEqual({ browser, os, kind })
  })
})

describe('lastSeenTone', () => {
  const now = Date.parse('2026-10-15T12:00:00Z')
  it('buckets by age', () => {
    expect(lastSeenTone('2026-10-15T11:58:00Z', now)).toBe('live')
    expect(lastSeenTone('2026-10-15T11:30:00Z', now)).toBe('recent')
    expect(lastSeenTone('2026-10-14T12:00:00Z', now)).toBe('idle')
  })
})
