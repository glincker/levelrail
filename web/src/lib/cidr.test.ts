import { describe, expect, it } from 'vitest'
import { checkSource } from './cidr'

describe('checkSource', () => {
  it('accepts addresses and CIDR blocks', () => {
    expect(checkSource('203.0.113.7')).toMatchObject({
      valid: true,
      kind: 'ipv4',
      rfc1918: false,
    })
    expect(checkSource(' 10.0.0.0/8 ')).toMatchObject({
      valid: true,
      kind: 'cidr4',
      rfc1918: true,
    })
    expect(checkSource('2001:db8::/32')).toMatchObject({
      valid: true,
      kind: 'cidr6',
    })
  })

  it('flags only the RFC1918 ranges as private', () => {
    expect(checkSource('172.16.0.1').rfc1918).toBe(true)
    expect(checkSource('172.32.0.1').rfc1918).toBe(false)
    expect(checkSource('192.168.1.0/24').rfc1918).toBe(true)
  })

  it('rejects malformed input', () => {
    for (const bad of [
      '',
      '256.1.1.1',
      '1.2.3',
      '1.2.3.4/33',
      '1.2.3.4/',
      '1.2.3.4/8/8',
      'abc',
      '01.2.3.4',
      '1::2::3',
    ]) {
      expect(checkSource(bad).valid, bad).toBe(false)
    }
  })
})
