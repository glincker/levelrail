import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { SettingsScopedSidebar } from './SettingsScopedSidebar'
import { SidebarProvider } from './ui/sidebar'

// No <RouterProvider> is mounted here, so Link and useRouterState (which
// both need real router context) are stubbed, same pattern as
// BrowseTemplatesFields.test.tsx.
let currentPathname = '/settings/account'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useRouterState: () => currentPathname,
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

function renderSidebar() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={queryClient}>
      <SidebarProvider>
        <SettingsScopedSidebar />
      </SidebarProvider>
    </QueryClientProvider>,
  )
}

describe('SettingsScopedSidebar', () => {
  beforeEach(() => {
    sessionStorage.clear()
    currentPathname = '/settings/account'
    // SidebarProvider's mobile-breakpoint detection (hooks/use-mobile.ts)
    // needs window.matchMedia, which jsdom does not implement.
    vi.stubGlobal(
      'matchMedia',
      vi.fn().mockReturnValue({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  it('renders every group open by default when there is no stored preference', () => {
    renderSidebar()

    // "Account" is both the group heading and an item title, so assert
    // via "Security" (an Account-only item) to avoid that ambiguity.
    expect(screen.getByText('Security')).toBeInTheDocument()
    expect(screen.getByText('Users')).toBeInTheDocument()
    expect(screen.getByText('Vault')).toBeInTheDocument()
    expect(screen.getByText('Audit log')).toBeInTheDocument()
  })

  it('toggles a group open and closed on heading click', async () => {
    const user = userEvent.setup()
    renderSidebar()

    expect(screen.getByText('Users')).toBeInTheDocument()

    await user.click(screen.getByText('Team'))
    expect(screen.queryByText('Users')).not.toBeInTheDocument()

    await user.click(screen.getByText('Team'))
    expect(screen.getByText('Users')).toBeInTheDocument()
  })

  it('respects a stored preference that previously collapsed a group', () => {
    // A real stored map always has every heading, because the in-memory
    // state it was saved from starts with every heading present.
    sessionStorage.setItem(
      'settings-sidebar-open-groups',
      JSON.stringify({
        Account: true,
        Team: false,
        'Git providers': true,
        'Notifications & status': true,
        'Storage & backups': true,
        'Registries & nodes': true,
        'Platform extras': true,
        Platform: true,
      }),
    )

    renderSidebar()

    expect(screen.queryByText('Users')).not.toBeInTheDocument()
    expect(screen.getByText('Security')).toBeInTheDocument()
  })

  it('filters items by search, expanding only matching groups', async () => {
    const user = userEvent.setup()
    renderSidebar()

    // All groups are open by default, so both are already visible.
    expect(screen.getByText('Vault')).toBeInTheDocument()
    expect(screen.getByText('Security')).toBeInTheDocument()

    await user.type(screen.getByLabelText('Search settings'), 'vault')

    expect(screen.getByText('Vault')).toBeInTheDocument()
    expect(screen.getByText('Platform extras')).toBeInTheDocument()
    expect(screen.queryByText('Security')).not.toBeInTheDocument()
    expect(screen.queryByText('Team')).not.toBeInTheDocument()

    await user.clear(screen.getByLabelText('Search settings'))

    expect(screen.getByText('Vault')).toBeInTheDocument()
    expect(screen.getByText('Security')).toBeInTheDocument()
  })

  it('matches on item description text too', async () => {
    const user = userEvent.setup()
    renderSidebar()

    await user.type(screen.getByLabelText('Search settings'), 'hashicorp vault')

    expect(screen.getByText('Vault')).toBeInTheDocument()
  })
})
