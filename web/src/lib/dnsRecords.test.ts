import { describe, expect, it } from 'vitest'
import {
  composeValue,
  filterRecords,
  parseValue,
  validateRecord,
  validateValue,
} from './dnsRecords'
import type { DnsCapabilities, DnsRecordSet } from '../types/dns'

const cf: DnsCapabilities = {
  proxied: true,
  routing: false,
  health_checks: false,
  apex_cname: true,
}
const r53: DnsCapabilities = {
  proxied: false,
  routing: true,
  health_checks: true,
  apex_cname: false,
}

describe('validateValue', () => {
  it.each([
    ['A', '203.0.113.1', null],
    ['A', '256.1.1.1', 'ipv4'],
    ['AAAA', '2001:db8::1', null],
    ['AAAA', '1.2.3.4', 'ipv6'],
    ['CNAME', 'host.example.net.', null],
    ['CNAME', 'not a host', 'host'],
    ['MX', '10 mail.example.com', null],
    ['MX', 'mail.example.com', 'priority'],
    ['SRV', '10 5 443 sip.example.com', null],
    ['SRV', '10 5 99999 sip.example.com', 'port'],
    ['CAA', '0 issue "letsencrypt.org"', null],
    ['CAA', '0 bogus "x"', 'caaTag'],
    ['TXT', 'v=spf1 -all', null],
    ['TXT', 'x'.repeat(4001), 'textTooLong'],
  ] as const)('%s %s', (type, value, want) => {
    expect(validateValue(type, value)).toBe(want)
  })
})

describe('compose and parse', () => {
  it('round trips every structured type', () => {
    expect(composeValue('MX', parseValue('MX', '10 mail.example.com'))).toBe(
      '10 mail.example.com',
    )
    expect(
      composeValue('SRV', parseValue('SRV', '1 2 443 sip.example.com')),
    ).toBe('1 2 443 sip.example.com')
    expect(
      composeValue('CAA', parseValue('CAA', '0 issuewild "letsencrypt.org"')),
    ).toBe('0 issuewild "letsencrypt.org"')
  })
})

describe('validateRecord', () => {
  const base: DnsRecordSet = {
    name: 'www',
    type: 'A',
    ttl: 300,
    values: ['1.2.3.4'],
  }
  it.each([
    ['valid', base, cf, []],
    [
      'apex NS',
      { ...base, type: 'NS', name: '@', values: ['ns1.x.net'] },
      cf,
      ['apexNs'],
    ],
    [
      'two CNAME values',
      { ...base, type: 'CNAME', values: ['a.net', 'b.net'] },
      cf,
      ['cnameSingle'],
    ],
    ['auto ttl on cloudflare', { ...base, ttl: 1 }, cf, []],
    ['auto ttl on route53', { ...base, ttl: 1 }, r53, ['ttl']],
    [
      'proxied TXT',
      { ...base, type: 'TXT', values: ['x'], proxied: true },
      cf,
      ['proxiedType'],
    ],
    [
      'weighted without id',
      { ...base, routing: 'weighted', weight: 5 },
      r53,
      ['setIdentifier'],
    ],
    [
      'weighted bad weight',
      { ...base, routing: 'weighted', set_identifier: 'a', weight: 300 },
      r53,
      ['routingWeight'],
    ],
    [
      'failover no role',
      { ...base, routing: 'failover', set_identifier: 'a' },
      r53,
      ['failoverRole'],
    ],
    ['nested wildcard', { ...base, name: 'a.*' }, cf, ['name']],
    ['empty', { ...base, values: [''] }, cf, ['noValues']],
  ] as const)('%s', (_, rec, caps, want) => {
    expect(validateRecord(rec as DnsRecordSet, caps)).toEqual(want)
  })
})

describe('filterRecords', () => {
  const recs: DnsRecordSet[] = [
    { name: 'www', type: 'A', ttl: 300, values: ['1.2.3.4'] },
    { name: '@', type: 'MX', ttl: 300, values: ['10 mail.example.com'] },
  ]
  it('filters by type and by name or value', () => {
    expect(filterRecords(recs, 'MX', '')).toHaveLength(1)
    expect(filterRecords(recs, '', '1.2.3')).toHaveLength(1)
    expect(filterRecords(recs, '', 'MAIL')).toHaveLength(1)
    expect(filterRecords(recs, 'A', 'mail')).toHaveLength(0)
  })
})
