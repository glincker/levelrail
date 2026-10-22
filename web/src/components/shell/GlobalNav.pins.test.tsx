import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { GlobalNav } from './GlobalNav'
import { SidebarProvider } from '@/components/ui/sidebar'
import { CommonI18n } from '@/test/commonI18n'
import { NAV_PINS_STORAGE_KEY } from '@/lib/navPins'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useRouterState: () => '/apps',
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
  }
})
vi.mock('@/lib/routeAvailability', () => ({
  useRouteAvailable: () => () => true,
}))
vi.mock('./useNavCounts', () => ({ useNavCounts: () => ({}) }))
vi.mock('@/hooks/useExperimental', () => ({
  useExperimentalFeatures: () => [],
}))
vi.mock('./ProjectsNavTree', () => ({ ProjectsNavItem: () => null }))
vi.mock('./GetStartedNavItem', () => ({ GetStartedNavItem: () => null }))

function renderNav() {
  return render(
    <CommonI18n>
      <SidebarProvider>
        <GlobalNav />
      </SidebarProvider>
    </CommonI18n>,
  )
}

describe('GlobalNav pins', () => {
  beforeEach(() => {
    window.localStorage.clear()
    window.matchMedia = vi.fn().mockReturnValue({
      matches: false,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })
  })

  it('hides the Pinned group when nothing is pinned', () => {
    renderNav()
    expect(screen.queryByText('Pinned')).toBeNull()
    expect(screen.getAllByRole('button', { name: /^Pin / })[0]).toHaveAttribute(
      'aria-pressed',
      'false',
    )
  })

  it('pins and unpins from the same button and persists', async () => {
    const user = userEvent.setup()
    renderNav()
    await user.click(screen.getByRole('button', { name: 'Pin Nodes' }))
    expect(screen.getByText('Pinned')).toBeTruthy()
    const pressed = screen.getAllByRole('button', { name: 'Pin Nodes' })
    expect(pressed).toHaveLength(2)
    expect(pressed[0]).toHaveAttribute('aria-pressed', 'true')
    expect(window.localStorage.getItem(NAV_PINS_STORAGE_KEY)).toBe('["nodes"]')
    await user.click(pressed[0] as HTMLElement)
    expect(screen.queryByText('Pinned')).toBeNull()
  })

  it('restores stored pins and drops unknown ids', () => {
    window.localStorage.setItem(NAV_PINS_STORAGE_KEY, '["nodes","gone"]')
    renderNav()
    expect(screen.getAllByRole('button', { name: 'Pin Nodes' })).toHaveLength(2)
  })
})
