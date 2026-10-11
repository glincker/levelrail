import { afterEach, describe, expect, it } from 'vitest'
import {
  parentCrumb,
  readListSearch,
  rememberListSearch,
  trafficCrumbs,
  type Crumb,
} from './trafficBreadcrumbs'

const ids = (crumbs: Crumb[] | null) => crumbs?.map((c) => c.id)
const labels = (crumbs: Crumb[] | null) =>
  crumbs?.map((c) => ('key' in c.label ? c.label.key : c.label.text))

afterEach(() => window.sessionStorage.clear())

describe('trafficCrumbs', () => {
  it('returns null outside the Traffic area', () => {
    expect(trafficCrumbs({ pathname: '/apps/web/overview' })).toBeNull()
    expect(trafficCrumbs({ pathname: '/' })).toBeNull()
  })

  it.each([
    ['/domains', 'nav.domains'],
    ['/dns', 'nav.dns'],
    ['/proxy', 'nav.proxy'],
    ['/network/proxy', 'nav.proxy'],
  ])('list page %s is a single current crumb', (pathname, label) => {
    const crumbs = trafficCrumbs({ pathname })
    expect(labels(crumbs)).toEqual([label])
    expect(crumbs?.[0]?.current).toBe(true)
  })

  it('builds Domains / domain for a domain page', () => {
    const crumbs = trafficCrumbs({ pathname: '/domains/shop.example.com' })
    expect(labels(crumbs)).toEqual(['nav.domains', 'shop.example.com'])
    expect(crumbs?.[0]?.to).toBe('/domains')
    expect(crumbs?.[1]?.current).toBe(true)
  })

  it('decodes an encoded wildcard domain', () => {
    const crumbs = trafficCrumbs({ pathname: '/domains/%2A.example.com' })
    expect(labels(crumbs)).toEqual(['nav.domains', '*.example.com'])
  })

  it('adds the tab and links the domain crumb', () => {
    const crumbs = trafficCrumbs({
      pathname: '/domains/shop.example.com/redirects',
    })
    expect(labels(crumbs)).toEqual([
      'nav.domains',
      'shop.example.com',
      'tab.redirects',
    ])
    expect(crumbs?.[1]?.to).toBe('/domains/$domain')
    expect(crumbs?.[1]?.params).toEqual({ domain: 'shop.example.com' })
  })

  it('ignores an unknown tab segment', () => {
    const crumbs = trafficCrumbs({ pathname: '/domains/a.example.com/zzz' })
    expect(ids(crumbs)).toEqual(['domains', 'domain'])
  })

  it('leads with the app when the visitor arrived from one', () => {
    const crumbs = trafficCrumbs({
      pathname: '/domains/shop.example.com',
      from: 'app:shop',
    })
    expect(labels(crumbs)).toEqual([
      'nav.apps',
      'shop',
      'nav.domains',
      'shop.example.com',
    ])
    expect(crumbs?.[2]?.to).toBe('/apps/$name/domains')
  })

  it('builds DNS / zone and DNS / zone / tab', () => {
    expect(labels(trafficCrumbs({ pathname: '/dns/example.com' }))).toEqual([
      'nav.dns',
      'example.com',
    ])
    expect(
      labels(trafficCrumbs({ pathname: '/dns/example.com/settings' })),
    ).toEqual(['nav.dns', 'example.com', 'tab.settings'])
  })

  it('names the wizard route instead of treating it as a zone', () => {
    expect(labels(trafficCrumbs({ pathname: '/dns/add' }))).toEqual([
      'nav.dns',
      'tab.add',
    ])
  })

  it('restores the remembered list search on the parent crumb', () => {
    const crumbs = trafficCrumbs({
      pathname: '/domains/shop.example.com',
      listSearch: { domains: { view: 'attention', q: 'api' } },
    })
    expect(crumbs?.[0]?.search).toEqual({ view: 'attention', q: 'api' })
  })
})

describe('parentCrumb', () => {
  it('is the nearest linked crumb', () => {
    const crumbs = trafficCrumbs({
      pathname: '/domains/shop.example.com/cache',
    }) as Crumb[]
    expect(parentCrumb(crumbs)?.id).toBe('domain')
  })

  it('is undefined on a list page', () => {
    const crumbs = trafficCrumbs({ pathname: '/dns' }) as Crumb[]
    expect(parentCrumb(crumbs)).toBeUndefined()
  })
})

describe('list search memory', () => {
  it('round-trips through sessionStorage', () => {
    rememberListSearch('dns', { q: 'ex' })
    expect(readListSearch('dns')).toEqual({ q: 'ex' })
    expect(readListSearch('domains')).toBeUndefined()
  })

  it('ignores corrupt storage', () => {
    window.sessionStorage.setItem('traffic.listSearch.dns', 'not json')
    expect(readListSearch('dns')).toBeUndefined()
  })
})
