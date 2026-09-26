import { describe, expect, it } from 'vitest'
import {
  APP_NAV_SECTIONS,
  GLOBAL_NAV_GROUPS,
  activeAppSection,
  allGlobalItems,
  isGlobalItemActive,
  resolveOpen,
} from './navModel'
import { isBoolRecord, readJson, writeJson } from './persist'

const EXPECTED_GLOBAL = [
  '/',
  '/status',
  '/apps',
  '/projects',
  '/databases',
  '/backups',
  '/nodes',
  '/domains',
  '/loadbalancers',
  '/models',
  '/pipelines',
  '/approvals',
  '/ai-assistant',
  '/settings',
  '/help',
]

const EXPECTED_APP = [
  'overview',
  'deploys',
  'source',
  'deploy-settings',
  'pipelines',
  'domains',
  'loadbalancer',
  'network',
  'environment',
  'services',
  'resources',
  'volumes',
  'health',
  'scheduled-tasks',
  'feature-flags',
  'integrations',
  'metrics',
  'logs',
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
    expect(GLOBAL_NAV_GROUPS.length).toBeLessThanOrEqual(7)
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
