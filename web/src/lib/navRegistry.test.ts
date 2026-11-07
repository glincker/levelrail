import { readdirSync, statSync } from 'node:fs'
import { join, relative, sep } from 'node:path'
import { describe, expect, it } from 'vitest'
import {
  FOLDED_GLOBAL_PAGES,
  GLOBAL_NAV_FOOTER,
  GLOBAL_NAV_GROUPS,
} from '@/components/shell/navModel'
import { settingsNavSections } from './settingsNav'
import { NAV_REGISTRY } from './navRegistry'

const ROUTES_DIR = join(__dirname, '..', 'routes')

function routeFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const full = join(dir, name)
    if (statSync(full).isDirectory()) return routeFiles(full)
    return /\.tsx$/.test(name) && !/\.test\.tsx$/.test(name) ? [full] : []
  })
}

// File name to URL path; param routes and the root are not pages to register.
function toPath(file: string): string | null {
  const rel = relative(ROUTES_DIR, file).split(sep).join('/')
  if (rel === '__root.tsx' || rel.includes('$')) return null
  const segs = rel
    .replace(/\.tsx$/, '')
    .split(/[/.]/)
    .filter((s) => s !== 'index' && s !== 'route')
    .map((s) => s.replace(/_$/, ''))
  return '/' + segs.join('/')
}

const SCANNED = [
  ...new Set(routeFiles(ROUTES_DIR).flatMap((f) => toPath(f) ?? [])),
].sort()

// The ratchet: this list may only shrink. Add a page to NAV_REGISTRY instead
// of here; each entry carries a one word reason.
const ALLOWLIST: Record<string, string> = {
  '/accept-invite': 'auth',
  '/accept-team-invite': 'auth',
  '/approve-sign-in': 'auth',
  '/forgot-password': 'auth',
  '/login': 'auth',
  '/reset-password': 'auth',
  '/sign-in-alert': 'auth',
  '/oauth/complete': 'callback',
  '/oauth/consent': 'callback',
  '/dashboards': 'redirect',
  '/databases/new': 'redirect',
  '/settings/janitor': 'redirect',
  '/settings/certificates': 'redirect',
  '/dev/ui': 'test',
  '/deploy': 'deeplink',
  '/apps/new': 'wizard',
  '/builds': 'tab',
  '/tags': 'tab',
  '/variables': 'tab',
  '/sources': 'tab',
  '/security/image-scanning': 'tab',
  '/nodes/bootstrap-scripts': 'tab',
  '/deploy-timeline': 'tab',
  '/costs': 'tab',
  '/cdn/caches': 'tab',
  '/cdn/redirects': 'tab',
  '/cdn/routing': 'tab',
  '/firewall/audit': 'tab',
  '/firewall/ports': 'tab',
  '/firewall/traffic': 'tab',
  '/firewall/rules': 'tab',
  '/firewall/rules/new': 'form',
  '/settings/platform/addresses': 'tab',
  '/settings/platform/dns-providers': 'tab',
  '/settings/platform/https': 'tab',
  '/settings/platform/ingress': 'tab',
}

const registered = new Set<string>(NAV_REGISTRY.map((e) => e.path))

describe('nav registry', () => {
  it('finds routes to check', () => {
    expect(SCANNED.length).toBeGreaterThan(50)
  })

  it('has a registry entry or an allowlist reason for every page route', () => {
    const missing = SCANNED.filter(
      (p) => !registered.has(p) && !(p in ALLOWLIST),
    )
    expect(
      missing,
      `Add these routes to NAV_REGISTRY in web/src/lib/navRegistry.ts (or, for an auth, callback, redirect or detail-style page, to ALLOWLIST with a one word reason): ${missing.join(', ')}`,
    ).toEqual([])
  })

  it('points every registry path at a real route', () => {
    const real = new Set(SCANNED)
    const bad = NAV_REGISTRY.filter((e) => !real.has(e.path)).map((e) => e.path)
    expect(bad, `Registry paths with no route file: ${bad.join(', ')}`).toEqual(
      [],
    )
  })

  it('keeps the allowlist to routes that exist and are not registered', () => {
    const real = new Set(SCANNED)
    const stale = Object.keys(ALLOWLIST).filter(
      (p) => !real.has(p) || registered.has(p),
    )
    expect(stale, `Remove from ALLOWLIST: ${stale.join(', ')}`).toEqual([])
  })

  it('has unique ids and paths and an allowlist reason of one word', () => {
    expect(new Set(NAV_REGISTRY.map((e) => e.id)).size).toBe(
      NAV_REGISTRY.length,
    )
    expect(registered.size).toBe(NAV_REGISTRY.length)
    for (const reason of Object.values(ALLOWLIST)) {
      expect(reason).toMatch(/^\w+$/)
    }
  })

  it('covers every sidebar and settings page with the same gate', () => {
    const byPath = new Map(NAV_REGISTRY.map((e) => [e.path, e] as const))
    const sidebar = [
      ...GLOBAL_NAV_GROUPS.flatMap((g) => g.items),
      ...GLOBAL_NAV_FOOTER,
    ]
    for (const item of sidebar) {
      const entry = byPath.get(item.to)
      expect(entry?.id, item.to).toBe(item.id)
      expect(entry?.sidebar).toBe(true)
      expect(entry?.gated).toBe(item.feature ?? null)
    }
    for (const section of settingsNavSections) {
      for (const item of section.items) {
        const entry = byPath.get(item.to as never)
        expect(entry?.settingsArea, item.to).toBe(section.id)
        expect(entry?.gated).toBe(item.feature ?? null)
      }
    }
    const listed = new Set<string>([
      ...sidebar.map((i) => i.to),
      ...FOLDED_GLOBAL_PAGES.map((i) => i.to),
      ...settingsNavSections.flatMap((s) => s.items.map((i) => i.to)),
    ])
    const extra = NAV_REGISTRY.filter((e) => !listed.has(e.path)).map(
      (e) => e.id,
    )
    expect(extra.sort()).toEqual(['get-started', 'migrate', 'welcome'])
  })
})
