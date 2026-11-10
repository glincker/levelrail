import { describe, expect, it } from 'vitest'
import {
  activeGlobalItemId,
  APP_NAV_SECTIONS,
  visibleAppSections,
  visibleGlobalGroups,
  GLOBAL_NAV_GROUPS,
  activeAppSection,
  allGlobalItems,
  defaultGroupOpen,
  isGlobalItemActive,
  isItemDisclosed,
  isSingleItemGroup,
  itemBadgeCount,
  pinnedGlobalItems,
  resolveOpen,
} from './navModel'
import { ROUTE_ENTRIES, navModelEntries } from '../commandPaletteData'
import { sidebarArea } from './sidebarArea'
import { isBoolRecord, readJson, writeJson } from './persist'

const EXPECTED_GLOBAL = [
  '/',
  '/status',
  '/apps',
  '/deployments',
  '/functions',
  '/projects',
  '/templates',
  '/tags',
  '/variables',
  '/sources',
  '/databases',
  '/backups',
  '/assets',
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
  '/mail',
  '/storage',
  '/inbox',
  '/settings/registry',
  '/settings/registry-credentials',
  '/settings/network-shares',
  '/settings/node-providers',
  '/settings/iam-policies',
  '/security',
  '/security/image-scanning',
  '/nodes/bootstrap-scripts',
  '/models',
  '/pipelines',
  '/approvals',
  '/alerts',
  '/ai-assistant',
  '/terminal',
  '/settings',
  '/help',
]

const FOLDED_PATHS = [
  '/status',
  '/projects',
  '/approvals',
  '/backups',
  '/dns',
  '/firewall',
  '/loadbalancers',
  '/cdn',
  '/network',
  '/settings/network-shares',
  '/settings/node-providers',
  '/settings/registry-credentials',
  '/settings/iam-policies',
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
  'containers',
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

  it('has one Dashboard entry and no separate Dashboards page', () => {
    const labels = allGlobalItems().map((i) => i.label)
    expect(labels).toContain('Dashboard')
    expect(labels).not.toContain('Dashboards')
    expect(
      allGlobalItems().some((i) => i.to === ('/dashboards' as string)),
    ).toBe(false)
  })

  it('keeps groups short', () => {
    expect(GLOBAL_NAV_GROUPS.length).toBeLessThanOrEqual(8)
  })

  it('shows the Overview rows ungrouped and twenty or fewer sidebar rows', () => {
    const primary = GLOBAL_NAV_GROUPS.filter((g) => g.ungrouped)
    expect(primary).toHaveLength(1)
    expect(primary[0]?.items.map((i) => i.label)).toEqual([
      'Dashboard',
      'Projects',
      'Inbox',
      'Alerts',
    ])
    const rows = GLOBAL_NAV_GROUPS.flatMap((g) => g.items)
    expect(rows.length).toBeLessThanOrEqual(20)
  })

  it('groups Domains, Traffic, Firewall and Mail under Network', () => {
    const net = GLOBAL_NAV_GROUPS.find((g) => g.id === 'traffic')
    expect(net?.label).toBe('Network')
    expect(net?.items.map((i) => i.label)).toEqual([
      'Domains',
      'Traffic',
      'Firewall',
      'Mail',
    ])
  })

  it('renders a single-item group as that item, not under a repeated header', () => {
    expect(isSingleItemGroup({ items: [1] })).toBe(true)
    for (const g of GLOBAL_NAV_GROUPS) {
      expect(g.ungrouped || !isSingleItemGroup(g), g.id).toBe(true)
    }
    expect(isSingleItemGroup({ items: [1, 2] })).toBe(false)
  })

  it('keeps every group at five items or fewer', () => {
    for (const g of GLOBAL_NAV_GROUPS) {
      expect(g.items.length, g.id).toBeLessThanOrEqual(5)
    }
  })

  it('opens every group on tall viewports and only the active one on short ones', () => {
    expect(defaultGroupOpen(900, false)).toBe(true)
    expect(defaultGroupOpen(800, false)).toBe(false)
    expect(defaultGroupOpen(800, true)).toBe(true)
  })

  it('aggregates domain and DNS attention onto Domains', () => {
    const domains = allGlobalItems().find((i) => i.id === 'domains')
    expect(
      domains &&
        itemBadgeCount(domains, { 'domains-attention': 2, 'dns-attention': 1 }),
    ).toBe(3)
  })

  it('hides Functions until there are functions, unless on its page', () => {
    const fn = allGlobalItems().find((i) => i.id === 'functions')
    expect(fn && isItemDisclosed(fn, {}, '/apps')).toBe(false)
    expect(fn && isItemDisclosed(fn, { functions: 2 }, '/apps')).toBe(true)
    expect(fn && isItemDisclosed(fn, {}, '/functions')).toBe(true)
  })

  it.each(FOLDED_PATHS)('keeps the main sidebar for %s', (path) => {
    expect(sidebarArea(path)).toBe('global')
  })

  it('hides route-gated items until the router has the route', () => {
    const labels = (has: boolean) =>
      visibleGlobalGroups(() => has)
        .flatMap((g) => g.items)
        .map((i) => i.label)
    expect(labels(true)).not.toContain('DNS')
    expect(labels(false)).toContain('Domains')
  })

  it.each([
    ['/', '/', true],
    ['/apps/web/logs', '/apps', true],
    ['/appsettings', '/apps', false],
    ['/status', '/', true],
    ['/network', '/network/proxy', true],
    ['/network/private', '/network/proxy', true],
    ['/network/private', '/firewall', false],
    ['/network/proxy', '/network/proxy', true],
    ['/firewall', '/firewall', true],
    ['/firewall', '/network/proxy', false],
  ])('active match %s vs %s', (path, to, want) => {
    const item = allGlobalItems().find((i) => i.to === to)
    expect(item && isGlobalItemActive(path, item)).toBe(want)
  })
})

describe('grouped nav entries', () => {
  const REMOVED = ['/deploy-timeline', '/builds', '/costs']
  const sidebarRows = () => GLOBAL_NAV_GROUPS.flatMap((g) => g.items)
  const item = (to: string) => sidebarRows().find((i) => i.to === to)

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
    ['/resources', '/usage', true],
    ['/approvals', '/pipelines', true],
    ['/settings/registry', '/settings/registry', true],
    ['/images', '/settings/registry', true],
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
  it('covers every existing app route exactly once in 4 sections', () => {
    expect(APP_NAV_SECTIONS).toHaveLength(4)
    const slugs = APP_NAV_SECTIONS.flatMap((s) => s.items.map((i) => i.slug))
    expect(slugs.sort()).toEqual([...EXPECTED_APP].sort())
  })

  it.each([
    ['/apps/web/deploys/abc/logs', 'deploy', 'deploys'],
    ['/apps/web/pipelines/x', 'deploy', 'pipelines'],
    ['/apps/web/network', 'traffic', 'network'],
    ['/apps/web/volumes', 'config', 'volumes'],
    ['/apps/web/exec', 'observe', 'exec'],
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

describe('palette entries derived from the nav model', () => {
  const entries = navModelEntries(ROUTE_ENTRIES, (id) => id)
  const tos = [...ROUTE_ENTRIES, ...entries].map((e) => e.to)

  it('makes templates, inbox, storage and the other pages reachable', () => {
    for (const to of [
      '/templates',
      '/inbox',
      '/storage',
      '/resources',
      '/resource-advisor',
      '/network',
      '/cdn',
      '/network/proxy',
      '/get-started',
      '/migrate',
    ]) {
      expect(tos, to).toContain(to)
    }
  })

  it('adds no duplicate destinations or keys', () => {
    const keys = [...ROUTE_ENTRIES, ...entries].map((e) => e.key)
    expect(new Set(keys).size).toBe(keys.length)
    expect(new Set(entries.map((e) => e.to)).size).toBe(entries.length)
    expect(
      entries
        .map((e) => e.to)
        .filter((to) => ROUTE_ENTRIES.some((r) => r.to === to)),
    ).toEqual([])
  })
})

describe('registry sidebar row', () => {
  it('links to a real page, not the /images redirect into Storage', () => {
    const reg = allGlobalItems().find((i) => i.id === 'registry')
    expect(reg?.to).toBe('/settings/registry')
  })
  it('is the active row on its page and the old images path', () => {
    const items = allGlobalItems()
    expect(activeGlobalItemId('/settings/registry', items)).toBe('registry')
    expect(activeGlobalItemId('/images', items)).toBe('registry')
  })
})

describe('activeGlobalItemId', () => {
  const items = allGlobalItems()
  it.each([
    ['/apps', 'apps'],
    ['/projects', 'projects'],
    ['/projects/p1', 'projects'],
    ['/templates', 'templates'],
  ])('%s activates only %s', (path, id) => {
    expect(activeGlobalItemId(path, items)).toBe(id)
  })
})
