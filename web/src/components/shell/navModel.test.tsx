import { describe, expect, it } from 'vitest'
import {
  APP_NAV_SECTIONS,
  visibleAppSections,
  visibleGlobalGroups,
  GLOBAL_NAV_GROUPS,
  activeAppSection,
  allGlobalItems,
  defaultGroupOpen,
  isGlobalItemActive,
  pinnedGlobalItems,
  resolveOpen,
} from './navModel'
import { ROUTE_ENTRIES } from '../commandPaletteData'
import { isBoolRecord, readJson, writeJson } from './persist'

const EXPECTED_GLOBAL = [
  '/',
  '/status',
  '/dashboards',
  '/apps',
  '/deployments',
  '/functions',
  '/projects',
  '/databases',
  '/backups',
  '/nodes',
  '/resource-advisor',
  '/usage',
  '/resources',
  '/network',
  '/network/proxy',
  '/network/private',
  '/cdn',
  '/domains',
  '/dns',
  '/firewall',
  '/loadbalancers',
  '/images',
  '/inbox',
  '/settings/registry',
  '/settings/registry-credentials',
  '/settings/network-shares',
  '/settings/node-providers',
  '/settings/iam-policies',
  '/security',
  '/models',
  '/pipelines',
  '/approvals',
  '/alerts',
  '/ai-assistant',
  '/terminal',
  '/settings',
  '/help',
]

const EXPECTED_APP = [
  'overview',
  'deploys',
  'source',
  'previews',
  'deploy-settings',
  'pipelines',
  'domains',
  'go-live',
  'loadbalancer',
  'streams',
  'network',
  'cache',
  'static-routes',
  'forms',
  'environment',
  'services',
  'resources',
  'volumes',
  'health',
  'scheduled-tasks',
  'feature-flags',
  'split-testing',
  'integrations',
  'metrics',
  'logs',
  'log-pipeline',
  'requests',
  'traces',
  'traffic',
  'errors',
  'alerts',
  'exec',
]

describe('global nav model', () => {
  it('lists every destination exactly once', () => {
    const tos = allGlobalItems()
      .map((i) => i.to)
      .sort()
    expect(tos).toEqual([...EXPECTED_GLOBAL].sort())
  })

  it('keeps groups short', () => {
    expect(GLOBAL_NAV_GROUPS.length).toBeLessThanOrEqual(8)
  })

  it('groups Domains, DNS, Proxy, Private networks, Firewall, Load balancers and CDN under Networking', () => {
    const net = GLOBAL_NAV_GROUPS.find((g) => g.id === 'traffic')
    expect(net?.label).toBe('Networking')
    expect(net?.items.map((i) => i.label)).toEqual([
      'Domains',
      'DNS',
      'Proxy',
      'Private networks',
      'Firewall',
      'Load balancers',
      'CDN and caches',
    ])
  })

  it('keeps every group at eight items or fewer', () => {
    for (const g of GLOBAL_NAV_GROUPS) {
      expect(g.items.length, g.id).toBeLessThanOrEqual(8)
    }
  })

  it('opens every group by default', () => {
    expect(defaultGroupOpen()).toBe(true)
  })

  it('hides route-gated items until the router has the route', () => {
    const labels = (has: boolean) =>
      visibleGlobalGroups(() => has)
        .flatMap((g) => g.items)
        .map((i) => i.label)
    expect(labels(false)).not.toContain('DNS')
    expect(labels(true)).toContain('DNS')
    expect(labels(false)).toContain('Domains')
  })

  it.each([
    ['/', '/', true],
    ['/apps/web/logs', '/apps', true],
    ['/appsettings', '/apps', false],
    ['/status', '/', false],
  ])('active match %s vs %s', (path, to, want) => {
    const item = allGlobalItems().find((i) => i.to === to)
    expect(item && isGlobalItemActive(path, item)).toBe(want)
  })
})

describe('grouped nav entries', () => {
  const REMOVED = ['/deploy-timeline', '/builds', '/costs']
  const item = (to: string) => allGlobalItems().find((i) => i.to === to)

  it.each([
    ['/deployments', '/deployments', true],
    ['/deploy-timeline', '/deployments', true],
    ['/deploy-timeline?x', '/deployments', true],
    ['/builds', '/deployments', true],
    ['/builds/something', '/deployments', true],
    ['/buildsx', '/deployments', false],
    ['/usage', '/usage', true],
    ['/costs', '/usage', true],
    ['/costs/x', '/usage', true],
    ['/costsx', '/usage', false],
    ['/images', '/deployments', false],
  ])('active match %s vs %s', (path, to, want) => {
    const it2 = item(to)
    expect(it2 && isGlobalItemActive(path, it2)).toBe(want)
  })

  it('keeps every removed destination reachable by alsoActive and the palette', () => {
    const also = allGlobalItems().flatMap((i) => i.alsoActive ?? [])
    const palette = ROUTE_ENTRIES.map((e) => e.to)
    for (const to of REMOVED) {
      expect(item(to), to).toBeUndefined()
      expect(also, to).toContain(to)
      expect(palette, to).toContain(to)
    }
  })

  it('ignores stale pins for removed entries', () => {
    const groups = GLOBAL_NAV_GROUPS
    const pinned = pinnedGlobalItems(groups, ['builds', 'costs', 'deployments'])
    expect(pinned.map((i) => i.id)).toEqual(['deployments'])
  })
})

describe('app nav model', () => {
  it('covers every existing app route exactly once in 6 sections', () => {
    expect(APP_NAV_SECTIONS).toHaveLength(6)
    const slugs = APP_NAV_SECTIONS.flatMap((s) => s.items.map((i) => i.slug))
    expect(slugs.sort()).toEqual([...EXPECTED_APP].sort())
  })

  it.each([
    ['/apps/web/deploys/abc/logs', 'deployments', 'deploys'],
    ['/apps/web/pipelines/x', 'deployments', 'pipelines'],
    ['/apps/web/network', 'traffic', 'network'],
    ['/apps/web/volumes', 'config', 'volumes'],
    ['/apps/web/exec', 'exec', 'exec'],
  ])('resolves %s', (path, section, item) => {
    const a = activeAppSection(path)
    expect([a?.section.id, a?.item.id]).toEqual([section, item])
  })

  it('returns nothing outside an app section', () => {
    expect(activeAppSection('/apps/web')).toBeUndefined()
  })
})

describe('collapsed state persistence', () => {
  it('stored value wins over the fallback', () => {
    expect(resolveOpen({ a: false }, 'a', true)).toBe(false)
    expect(resolveOpen({}, 'a', true)).toBe(true)
  })

  it('round-trips through storage and ignores garbage', () => {
    writeJson('k', { a: false, b: true })
    expect(readJson('k', {}, isBoolRecord)).toEqual({ a: false, b: true })
    window.localStorage.setItem('k', '{"a":"x"}')
    expect(readJson('k', {}, isBoolRecord)).toEqual({})
    window.localStorage.setItem('k', 'not json')
    expect(readJson('k', { z: true }, isBoolRecord)).toEqual({ z: true })
  })
})

describe('static routing rules nav item', () => {
  const slugs = (isStatic: boolean) =>
    visibleAppSections(isStatic).flatMap((s) => s.items.map((i) => i.slug))

  it('appears only for static sites', () => {
    expect(slugs(true)).toContain('static-routes')
    expect(slugs(false)).not.toContain('static-routes')
  })
})
