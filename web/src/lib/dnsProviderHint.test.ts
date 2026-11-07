import { describe, expect, it } from 'vitest'
import { PROVIDER_DNS_URL, detectNsProvider } from './dnsProviderHint'

describe('detectNsProvider', () => {
  it.each([
    [['ada.ns.cloudflare.com'], 'cloudflare'],
    [['ns-123.awsdns-45.com.'], 'route53'],
    [['ns01.domaincontrol.com'], 'godaddy'],
    [['dns1.registrar-servers.com'], 'namecheap'],
  ])('recognises %j as %s', (servers, expected) => {
    expect(detectNsProvider(servers)).toBe(expected)
  })

  it('matches case-insensitively and uses the first known server', () => {
    expect(
      detectNsProvider(['ns.unknown.example', 'BOB.NS.CLOUDFLARE.COM']),
    ).toBe('cloudflare')
  })

  it('falls back to generic for unknown or empty lists', () => {
    expect(detectNsProvider(['ns1.example.org'])).toBe('generic')
    expect(detectNsProvider([])).toBe('generic')
  })

  it('has a dashboard link for every named provider', () => {
    expect(Object.keys(PROVIDER_DNS_URL).sort()).toEqual([
      'cloudflare',
      'godaddy',
      'google',
      'namecheap',
      'route53',
    ])
  })
})
