import { describe, expect, it, vi } from 'vitest'
import type { TFunction } from 'i18next'
import type { Domain } from '@/queries/domains'
import {
  buildTrafficPaletteItems,
  domainFromPath,
  type TrafficPaletteInput,
} from './trafficPaletteItems'
import { testI18n } from './traffic/testUtils'

const t: TFunction<'traffic'> = testI18n.getFixedT('en', 'traffic')

function domain(over: Partial<Domain> = {}): Domain {
  return {
    domain: 'shop.example.com',
    service_name: 'shop',
    waf_enabled: false,
    has_redirect: false,
    maintenance_enabled: false,
    has_basic_auth: false,
    ...over,
  }
}

function input(over: Partial<TrafficPaletteInput> = {}): TrafficPaletteInput {
  return {
    domains: [
      domain(),
      domain({ domain: 'api.example.com', service_name: 'api' }),
    ],
    query: '',
    routeAvailable: () => false,
    t,
    navigate: vi.fn(),
    verify: vi.fn(),
    ...over,
  }
}

const keys = (items: { key: string }[]) => items.map((i) => i.key)

describe('domainFromPath', () => {
  it.each([
    ['/domains/shop.example.com', 'shop.example.com'],
    ['/domains/shop.example.com/redirects', 'shop.example.com'],
    ['/domains/%2A.example.com', '*.example.com'],
    ['/domains', undefined],
    ['/apps/web/domains', undefined],
  ])('%s', (path, want) => {
    expect(domainFromPath(path)).toBe(want)
  })
})

describe('buildTrafficPaletteItems base items', () => {
  it('always offers add domain, proxy and every domain', () => {
    const { base } = buildTrafficPaletteItems(input())
    expect(keys(base)).toEqual(
      expect.arrayContaining([
        'action-add-domain',
        'nav-proxy',
        'domain-shop.example.com',
        'domain-api.example.com',
      ]),
    )
  })

  it('hides the DNS and settings entries until their routes exist', () => {
    const missing = keys(buildTrafficPaletteItems(input()).base)
    expect(missing).not.toContain('nav-dns')
    expect(missing).not.toContain('settings-domains')
    const present = keys(
      buildTrafficPaletteItems(input({ routeAvailable: () => true })).base,
    )
    expect(present).toEqual(
      expect.arrayContaining(['nav-dns', 'settings-domains']),
    )
  })

  it('matches a domain by its app name through keywords', () => {
    const { base } = buildTrafficPaletteItems(input())
    expect(
      base.find((i) => i.key === 'domain-shop.example.com')?.keywords,
    ).toBe('shop')
  })

  it('adds a status badge only when the status is actually known', () => {
    const { base } = buildTrafficPaletteItems(
      input({
        domains: [
          domain({ maintenance_enabled: true }),
          domain({ domain: 'plain.example.com' }),
        ],
      }),
    )
    expect(
      base.find((i) => i.key === 'domain-shop.example.com')?.badge,
    ).toBeTruthy()
    expect(
      base.find((i) => i.key === 'domain-plain.example.com')?.badge,
    ).toBeUndefined()
  })

  it('puts domain actions first when a domain page is open', () => {
    const verify = vi.fn()
    const { base } = buildTrafficPaletteItems(
      input({
        currentDomain: 'shop.example.com',
        routeAvailable: (to) => to === '/domains/$domain',
        verify,
      }),
    )
    const ctx = base.filter((i) => i.group === 'Domain actions')
    expect(keys(ctx)).toEqual([
      'domain-action-verify-shop.example.com',
      'domain-action-doctor-shop.example.com',
    ])
    ctx[0]?.run()
    expect(verify).toHaveBeenCalledWith('shop', 'shop.example.com')
  })

  it('skips the doctor action while its route is missing', () => {
    const { base } = buildTrafficPaletteItems(
      input({ currentDomain: 'shop.example.com' }),
    )
    expect(keys(base)).not.toContain('domain-action-doctor-shop.example.com')
  })

  it('add domain opens the add flow through URL state', () => {
    const navigate = vi.fn()
    const { base } = buildTrafficPaletteItems(input({ navigate }))
    base.find((i) => i.key === 'action-add-domain')?.run()
    expect(navigate).toHaveBeenCalledWith({
      to: '/domains',
      search: { add: '1' },
    })
  })
})

describe('buildTrafficPaletteItems search items', () => {
  it('emits nothing for a one character query', () => {
    expect(buildTrafficPaletteItems(input({ query: 's' })).search).toEqual([])
  })

  it('offers verify and DNS record entries for matching domains', () => {
    const { search } = buildTrafficPaletteItems(input({ query: 'shop' }))
    expect(keys(search)).toEqual([
      'domain-action-verify-shop.example.com',
      'dns-records-shop.example.com',
    ])
    expect(search.map((i) => i.group)).toEqual([
      'Domain actions',
      'DNS records',
    ])
  })

  it('ranks a label match above an app-name-only match', () => {
    const { search } = buildTrafficPaletteItems(
      input({
        query: 'api',
        domains: [
          domain({ domain: 'web.example.com', service_name: 'api' }),
          domain({ domain: 'api.example.com', service_name: 'web' }),
        ],
      }),
    )
    expect(search[0]?.key).toBe('domain-action-verify-api.example.com')
  })

  it('caps verify at 3 and DNS records at 5', () => {
    const many = Array.from({ length: 8 }, (_, i) =>
      domain({ domain: `d${String(i)}.example.com`, service_name: 'shop' }),
    )
    const { search } = buildTrafficPaletteItems(
      input({ query: 'example', domains: many }),
    )
    expect(search.filter((i) => i.group === 'Domain actions')).toHaveLength(3)
    expect(search.filter((i) => i.group === 'DNS records')).toHaveLength(5)
  })
})
