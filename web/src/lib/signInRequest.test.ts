import { describe, expect, it } from 'vitest'
import {
  formatCountdown,
  ipScope,
  isValidMatch,
  parseDeviceLabel,
  sanitizeDigits,
  secondsLeft,
} from './signInRequest'

describe('parseDeviceLabel', () => {
  it('splits browser and os', () => {
    expect(parseDeviceLabel('Chrome on macOS')).toMatchObject({
      kind: 'browser',
      browser: 'Chrome',
      os: 'macOS',
    })
  })
  it('recognises client labels', () => {
    expect(parseDeviceLabel('Command line client').kind).toBe('cli')
    expect(parseDeviceLabel('MCP client').kind).toBe('mcp')
    expect(parseDeviceLabel('Unknown browser').kind).toBe('unknown')
  })
})

describe('ipScope', () => {
  it('flags loopback and private ranges', () => {
    expect(ipScope('127.0.0.1')).toBe('this-computer')
    expect(ipScope('::1')).toBe('this-computer')
    expect(ipScope('192.168.1.4')).toBe('local-network')
    expect(ipScope('10.0.0.9')).toBe('local-network')
    expect(ipScope('172.20.1.1')).toBe('local-network')
    expect(ipScope('fd12::1')).toBe('local-network')
    expect(ipScope('172.32.0.1')).toBe('public')
    expect(ipScope('203.0.113.7')).toBe('public')
  })
})

describe('countdown', () => {
  it('counts down and clamps at zero', () => {
    const exp = '2026-10-21T10:05:00Z'
    const t0 = Date.parse('2026-10-21T10:03:30Z')
    expect(secondsLeft(exp, t0)).toBe(90)
    expect(formatCountdown(90)).toBe('1:30')
    expect(formatCountdown(5)).toBe('0:05')
    expect(secondsLeft(exp, Date.parse('2026-10-21T10:06:00Z'))).toBe(0)
  })
})

describe('match input', () => {
  it('validates two digits without a leading zero', () => {
    expect(isValidMatch('42')).toBe(true)
    expect(isValidMatch('4')).toBe(false)
    expect(isValidMatch('04')).toBe(false)
    expect(sanitizeDigits(' 4 2x9')).toBe('42')
  })
})
