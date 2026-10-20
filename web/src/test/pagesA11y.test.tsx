import type { AnyRoute } from '@tanstack/react-router'
import { describe, expect, it, vi } from 'vitest'
import { pageViolations } from './pageHarness'

vi.mock('@/components/HelpLink', () => ({ HelpLink: () => null }))

interface Page {
  name: string
  load: () => Promise<{ Route: AnyRoute }>
  path: string
  entry: string
}

// Parent layout routes own the page h1, so readiness is just rendered content.
const hasContent = (root: HTMLElement) =>
  (root.textContent ?? '').trim().length > 40

const pages: Page[] = [
  {
    name: 'dashboard home',
    load: () => import('../routes/index'),
    path: '/',
    entry: '/',
  },
  {
    name: 'apps list',
    load: () => import('../routes/apps/index'),
    path: '/apps/',
    entry: '/apps/',
  },
  {
    name: 'app overview',
    load: () => import('../routes/apps/$name/overview'),
    path: '/apps/$name/overview',
    entry: '/apps/web/overview',
  },
  {
    name: 'deploys',
    load: () => import('../routes/deployments'),
    path: '/deployments',
    entry: '/deployments',
  },
  {
    name: 'deploy detail',
    load: () => import('../routes/apps/$name/deploys/$deployId/logs'),
    path: '/apps/$name/deploys/$deployId/logs',
    entry: '/apps/web/deploys/d1/logs',
  },
  {
    name: 'app logs',
    load: () => import('../routes/apps/$name/logs'),
    path: '/apps/$name/logs',
    entry: '/apps/web/logs',
  },
  {
    name: 'databases list',
    load: () => import('../routes/databases/index'),
    path: '/databases/',
    entry: '/databases/',
  },
  {
    name: 'database overview',
    load: () => import('../routes/databases/$name/overview'),
    path: '/databases/$name/overview',
    entry: '/databases/pg/overview',
  },
  {
    name: 'domains',
    load: () => import('../routes/domains/index'),
    path: '/domains/',
    entry: '/domains/',
  },
  {
    name: 'nodes',
    load: () => import('../routes/nodes/index'),
    path: '/nodes/',
    entry: '/nodes/',
  },
  {
    name: 'settings general',
    load: () => import('../routes/settings/general'),
    path: '/settings/general',
    entry: '/settings/general',
  },
  {
    name: 'settings security',
    load: () => import('../routes/settings/security'),
    path: '/settings/security',
    entry: '/settings/security',
  },
  {
    name: 'account and passkeys',
    load: () => import('../routes/settings/account'),
    path: '/settings/account',
    entry: '/settings/account',
  },
  {
    name: 'templates catalog',
    load: () => import('../routes/templates/index'),
    path: '/templates/',
    entry: '/templates/',
  },
  {
    name: 'plugins',
    load: () => import('../routes/settings/plugins'),
    path: '/settings/plugins',
    entry: '/settings/plugins',
  },
  {
    name: 'backups',
    load: () => import('../routes/backups/index'),
    path: '/backups/',
    entry: '/backups/',
  },
  {
    name: 'alerts',
    load: () => import('../routes/alerts'),
    path: '/alerts',
    entry: '/alerts',
  },
  {
    name: 'security center',
    load: () => import('../routes/security/index'),
    path: '/security/',
    entry: '/security/',
  },
]

describe('page accessibility', () => {
  it.each(pages)(
    '$name has no static accessibility violations',
    async ({ load, path, entry }) => {
      const { Route } = await load()
      const found = await pageViolations(Route, hasContent, { path, entry })
      expect(found).toEqual([])
    },
    60000,
  )

  it('command palette dialog is named and its input labelled', async () => {
    const { CommandPalette } = await import('../components/CommandPalette')
    const found = await pageViolations(
      () => <CommandPalette open onOpenChange={() => {}} />,
      (root) => root.querySelector('[role="dialog"]') !== null,
    )
    expect(found).toEqual([])
    const dialog = document.querySelector('[role="dialog"]')
    expect(
      dialog?.getAttribute('aria-label') ??
        dialog?.getAttribute('aria-labelledby'),
    ).toBeTruthy()
  }, 60000)
})
