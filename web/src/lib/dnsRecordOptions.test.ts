import { describe, expect, it } from 'vitest'
import type { DomainCheckPanelData } from '../components/DomainDnsCheck'
import { dnsRecordOptions } from './dnsRecordOptions'

function panel(over: Partial<DomainCheckPanelData>): DomainCheckPanelData {
  return over as DomainCheckPanelData
}

describe('dnsRecordOptions', () => {
  it('returns nothing without an expected host', () => {
    expect(dnsRecordOptions(panel({}))).toEqual([])
  })

  it('offers only an A record for a raw IPv4 host', () => {
    expect(dnsRecordOptions(panel({ expected_host: '203.0.113.7' }))).toEqual([
      { type: 'A', value: '203.0.113.7' },
    ])
  })

  it('offers only an AAAA record for a raw IPv6 host', () => {
    expect(dnsRecordOptions(panel({ expected_host: '2001:db8::1' }))).toEqual([
      { type: 'AAAA', value: '2001:db8::1' },
    ])
  })

  it('puts CNAME first for a hostname, then A and AAAA alternatives', () => {
    const options = dnsRecordOptions(
      panel({
        expected_host: 'lr.example.com',
        expected_ipv4: ['203.0.113.7'],
        expected_ipv6: ['2001:db8::1'],
      }),
    )
    expect(options.map((o) => o.type)).toEqual(['CNAME', 'A', 'AAAA'])
  })
})
