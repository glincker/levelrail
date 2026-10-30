import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import settingsEn from '../locales/en/settings.json'
import { SettingsScopedSidebar } from './SettingsScopedSidebar'
import { SidebarProvider } from './ui/sidebar'
import { createTestQueryClient } from '@/test/testQueryClient'

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

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

function renderSidebar() {
  const queryClient = createTestQueryClient()
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <SidebarProvider>
          <SettingsScopedSidebar />
        </SidebarProvider>
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('SettingsScopedSidebar', () => {
  beforeEach(() => {
    localStorage.clear()
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

  it('opens only the group holding the current page by default', () => {
    renderSidebar()

    expect(screen.getByText('People and access')).toBeInTheDocument()
    expect(screen.queryByText('Users')).not.toBeInTheDocument()
    expect(screen.queryByText('Vault')).not.toBeInTheDocument()
  })

  it('toggles a group open and closed on heading click', async () => {
    const user = userEvent.setup()
    renderSidebar()

    expect(screen.queryByText('Users')).not.toBeInTheDocument()

    await user.click(screen.getByText('People and access'))
    expect(screen.getByText('Users')).toBeInTheDocument()

    await user.click(screen.getByText('People and access'))
    expect(screen.queryByText('Users')).not.toBeInTheDocument()
  })

  it('respects a stored preference that previously opened a group', () => {
    localStorage.setItem(
      'settings-sidebar-open-groups:v2',
      JSON.stringify({ access: true }),
    )

    renderSidebar()

    expect(screen.getByText('Users')).toBeInTheDocument()
  })

  it('keeps Platform active on every platform page', () => {
    currentPathname = '/settings/platform/dns-providers'
    renderSidebar()

    expect(screen.getByRole('link', { name: 'Platform' })).toHaveAttribute(
      'data-active',
      'true',
    )
  })

  it('renders and toggles with storage that throws', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('blocked')
    })
    const user = userEvent.setup()
    currentPathname = '/settings/platform'
    renderSidebar()

    expect(screen.getByRole('link', { name: 'Platform' })).toBeInTheDocument()
    expect(screen.queryByText('Users')).not.toBeInTheDocument()
    await user.click(screen.getByText('People and access'))
    expect(screen.getByText('Users')).toBeInTheDocument()
  })

  it('filters items by search, expanding only matching groups', async () => {
    const user = userEvent.setup()
    renderSidebar()

    expect(screen.queryByText('Vault')).not.toBeInTheDocument()

    await user.type(screen.getByLabelText('Search settings'), 'vault')

    expect(screen.getByText('Vault')).toBeInTheDocument()
    expect(screen.getByText('Security')).toBeInTheDocument()
    expect(screen.queryByText('Domains and TLS')).not.toBeInTheDocument()

    await user.clear(screen.getByLabelText('Search settings'))

    expect(screen.queryByText('Vault')).not.toBeInTheDocument()
  })

  it('matches on item description text too', async () => {
    const user = userEvent.setup()
    renderSidebar()

    await user.type(screen.getByLabelText('Search settings'), 'hashicorp vault')

    expect(screen.getByText('Vault')).toBeInTheDocument()
  })
})
