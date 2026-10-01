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

  it('starts with the active route group open and others collapsed', () => {
    renderSidebar()

    // "Account" is both the group heading and an item title, so assert
    // via "Security" (an Account-only item) to avoid that ambiguity.
    expect(screen.getByText('Security')).toBeInTheDocument()

    expect(screen.getByText('Team')).toBeInTheDocument()
    expect(screen.queryByText('Users')).not.toBeInTheDocument()
  })

  it('toggles a group open and closed on heading click', async () => {
    const user = userEvent.setup()
    renderSidebar()

    expect(screen.queryByText('Users')).not.toBeInTheDocument()

    await user.click(screen.getByText('Team'))
    expect(screen.getByText('Users')).toBeInTheDocument()

    await user.click(screen.getByText('Team'))
    expect(screen.queryByText('Users')).not.toBeInTheDocument()
  })

  it('filters items by search, expanding only matching groups', async () => {
    const user = userEvent.setup()
    renderSidebar()

    expect(screen.queryByText('Vault')).not.toBeInTheDocument()
    expect(screen.getByText('Security')).toBeInTheDocument()

    await user.type(screen.getByLabelText('Search settings'), 'vault')

    expect(screen.getByText('Vault')).toBeInTheDocument()
    expect(screen.getByText('Platform extras')).toBeInTheDocument()
    expect(screen.queryByText('Security')).not.toBeInTheDocument()
    expect(screen.queryByText('Team')).not.toBeInTheDocument()

    await user.clear(screen.getByLabelText('Search settings'))

    expect(screen.queryByText('Vault')).not.toBeInTheDocument()
    expect(screen.getByText('Security')).toBeInTheDocument()
  })

  it('matches on item description text too', async () => {
    const user = userEvent.setup()
    renderSidebar()

    await user.type(screen.getByLabelText('Search settings'), 'hashicorp vault')

    expect(screen.getByText('Vault')).toBeInTheDocument()
  })
})
