import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { cleanup, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { CommonI18n } from '@/test/commonI18n'
import { NavTabStrip } from './NavTabStrip'

const experimental = vi.hoisted(() => ({ enabled: [] as string[] }))
const routes = vi.hoisted(() => ({ has: true }))
vi.mock('@/hooks/useExperimental', () => ({
  useExperimentalFeatures: () => experimental.enabled,
}))
vi.mock('@/lib/routeAvailability', () => ({
  useRouteAvailable: () => () => routes.has,
}))

const router = vi.hoisted(() => ({ pathname: '/' }))

vi.mock('@tanstack/react-router', () => ({
  useRouterState: ({
    select,
  }: {
    select: (s: { location: { pathname: string } }) => string
  }) => select({ location: { pathname: router.pathname } }),
  Link: ({
    children,
    to,
    ...rest
  }: {
    children?: ReactNode
    to?: string
  } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a href={to} {...rest}>
      {children}
    </a>
  ),
}))

function current() {
  return screen
    .getAllByRole('link')
    .filter((l) => l.getAttribute('aria-current') === 'page')
    .map((l) => l.textContent)
}

function labels() {
  return screen.queryAllByRole('link').map((l) => l.textContent)
}

function renderAt(path: string) {
  router.pathname = path
  render(
    <CommonI18n>
      <NavTabStrip />
    </CommonI18n>,
  )
}

describe('NavTabStrip', () => {
  beforeEach(() => {
    experimental.enabled = []
    routes.has = true
  })

  it('shows sibling pages and marks the current one', () => {
    renderAt('/projects')
    expect(labels()).toEqual([
      'Apps',
      'Projects',
      'Templates',
      'Tags',
      'Variables',
      'Sources',
    ])
    expect(current()).toEqual(['Projects'])
  })

  it('renders nothing outside a folded group', () => {
    renderAt('/apps/web')
    expect(labels()).toEqual([])
    cleanup()
    renderAt('/inbox')
    expect(labels()).toEqual([])
  })

  it('drops a strip left with a single tab', () => {
    routes.has = false
    renderAt('/domains')
    expect(labels()).toEqual([])
  })

  it('shows DNS with a route and Mail only when unlocked', () => {
    renderAt('/domains')
    expect(labels()).toEqual(['Domains', 'DNS'])
    cleanup()
    experimental.enabled = ['mail']
    renderAt('/domains')
    expect(labels()).toEqual(['Domains', 'DNS', 'Mail'])
  })

  it('lists the security pages and marks only the longest match', () => {
    renderAt('/security/image-scanning')
    expect(labels()).toEqual([
      'Security center',
      'Keys and tokens',
      'Team',
      'Audit log',
      'Image scanning',
      'IAM policies',
    ])
    expect(current()).toEqual(['Image scanning'])
    cleanup()
    renderAt('/settings/audit-log')
    expect(current()).toEqual(['Audit log'])
  })

  it('adds bootstrap scripts to the server tabs', () => {
    renderAt('/nodes/bootstrap-scripts')
    expect(labels()).toEqual(['Nodes', 'Cloud providers', 'Bootstrap scripts'])
    expect(current()).toEqual(['Bootstrap scripts'])
  })

  it('leaves usage and costs to their own header strip', () => {
    renderAt('/usage')
    expect(labels()).toEqual([])
    cleanup()
    renderAt('/resources')
    expect(labels()).toEqual(['Usage', 'Costs', 'Resources'])
  })
})
