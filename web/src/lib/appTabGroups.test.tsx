import { describe, expect, it } from 'vitest'
import { readdirSync } from 'node:fs'
import { join } from 'node:path'
import {
  APP_TAB_GROUPS,
  groupForSlug,
  groupTabs,
  splitForNarrow,
} from './appTabGroups'
import { APP_NAV_SECTIONS } from '../components/shell/navModel'

const routeDir = join(__dirname, '../routes/apps/$name')
const routeSlugs = readdirSync(routeDir)
  .map((f) => f.replace(/\.tsx$/, ''))
  .filter((f) => !['index', 'forms', 'deploys', 'pipelines'].includes(f))

const navSlugs = APP_NAV_SECTIONS.flatMap((s) => s.items.map((i) => i.slug))

describe('app tab groups', () => {
  it('lists each configured slug in exactly one group', () => {
    const all = APP_TAB_GROUPS.flatMap((g) => g.slugs)
    expect(new Set(all).size).toBe(all.length)
  })

  it('places every nav tab and route in exactly one group', () => {
    const grouped = groupTabs(
      navSlugs.map((slug) => ({ slug })),
      { showAll: true },
    )
    const placed = grouped.flatMap((g) => g.items.map((i) => i.slug))
    expect(placed.sort()).toEqual([...navSlugs].sort())
    for (const slug of [...navSlugs, ...routeSlugs]) {
      expect(APP_TAB_GROUPS.map((g) => g.id)).toContain(groupForSlug(slug))
    }
  })

  it('covers every route file with a nav tab', () => {
    for (const slug of routeSlugs) expect(navSlugs).toContain(slug)
  })

  it('sends an unlisted tab to advanced', () => {
    expect(groupForSlug('brand-new-tab')).toBe('advanced')
  })

  it('keeps the basics order from the config', () => {
    const basics = groupTabs(navSlugs.map((slug) => ({ slug })))[0]
    expect(basics?.items.map((i) => i.slug)).toEqual([
      'overview',
      'deploys',
      'environment',
      'logs',
    ])
  })

  it('hides non-applicable tabs for static sites unless show all is on', () => {
    const tabs = ['overview', 'volumes', 'exec'].map((slug) => ({ slug }))
    const slugs = (showAll: boolean) =>
      groupTabs(tabs, { isStatic: true, showAll }).flatMap((g) =>
        g.items.map((i) => i.slug),
      )
    expect(slugs(false)).toEqual(['overview'])
    expect(slugs(true)).toEqual(['overview', 'volumes', 'exec'])
  })
})

describe('narrow screen collapse', () => {
  const groups = APP_TAB_GROUPS.map((g) => ({ id: g.id }))

  it('keeps every group on wide screens', () => {
    const r = splitForNarrow(groups, 'network', false)
    expect(r.visible).toHaveLength(5)
    expect(r.more).toEqual([])
  })

  it('keeps only the active group and moves the rest under More', () => {
    const r = splitForNarrow(groups, 'observe', true)
    expect(r.visible.map((g) => g.id)).toEqual(['observe'])
    expect(r.more.map((g) => g.id)).toEqual([
      'basics',
      'network',
      'data',
      'advanced',
    ])
  })

  it('falls back to the first group when none is active', () => {
    expect(splitForNarrow(groups, undefined, true).visible[0]?.id).toBe(
      'basics',
    )
  })
})
