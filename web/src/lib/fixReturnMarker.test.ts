import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  clearFixMarker,
  fixMarkerTtlMs,
  readFixMarker,
  writeFixMarker,
} from './fixReturnMarker'

const marker = {
  appName: 'web',
  deployId: 'd1',
  labelKey: 'environment' as const,
  at: 1_000_000,
}

describe('fixReturnMarker', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    window.sessionStorage.clear()
  })

  it('round trips a fresh marker for the same app', () => {
    writeFixMarker(marker)
    expect(readFixMarker('web', marker.at + 1000)).toEqual(marker)
  })

  it('ignores a marker for another app', () => {
    writeFixMarker(marker)
    expect(readFixMarker('api', marker.at + 1000)).toBeNull()
  })

  it('expires after the TTL', () => {
    writeFixMarker(marker)
    expect(readFixMarker('web', marker.at + fixMarkerTtlMs() + 1)).toBeNull()
  })

  it('returns null for malformed JSON or wrong shape', () => {
    window.sessionStorage.setItem('levelrail.fixReturnMarker', '{nope')
    expect(readFixMarker('web', marker.at)).toBeNull()
    window.sessionStorage.setItem('levelrail.fixReturnMarker', '{"appName":1}')
    expect(readFixMarker('web', marker.at)).toBeNull()
  })

  it('clears the marker', () => {
    writeFixMarker(marker)
    clearFixMarker()
    expect(readFixMarker('web', marker.at)).toBeNull()
  })

  it('does not throw when storage is unavailable', () => {
    const boom = () => {
      throw new Error('blocked')
    }
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(boom)
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(boom)
    vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(boom)
    expect(() => {
      writeFixMarker(marker)
    }).not.toThrow()
    expect(() => {
      clearFixMarker()
    }).not.toThrow()
    expect(readFixMarker('web', marker.at)).toBeNull()
  })
})
