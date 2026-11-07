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

  it('opens the group holding the current page and keeps the rest closed', () => {
    renderSidebar()

    expect(screen.getByRole('button', { name: /Workspace/ })).toHaveAttribute(
      'aria-expanded',
      'true',
    )
    expect(
      screen.getByRole('button', { name: /People and access/ }),
    ).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('Vault')).not.toBeInTheDocument()
  })

  it('expands another group in place without losing the open one', async () => {
    const user = userEvent.setup()
    renderSidebar()

    await user.click(screen.getByRole('button', { name: /^Security/ }))

    expect(screen.getByText('Vault')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Workspace/ })).toHaveAttribute(
      'aria-expanded',
      'true',
    )
    await user.click(screen.getByRole('button', { name: /^Security/ }))
    expect(screen.queryByText('Vault')).not.toBeInTheDocument()
  })

  it('lists every group closed on the settings hub', () => {
    currentPathname = '/settings'
    renderSidebar()

    expect(screen.getByText('People and access')).toBeInTheDocument()
    expect(screen.queryByText('Users')).not.toBeInTheDocument()
  })

  it('keeps Platform active on every platform page', () => {
    currentPathname = '/settings/platform/dns-providers'
    renderSidebar()

    expect(screen.getByRole('link', { name: 'Platform' })).toHaveAttribute(
      'data-active',
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

  it('shows a flat result list across areas while searching', async () => {
    const user = userEvent.setup()
    renderSidebar()

    await user.type(screen.getByLabelText('Search settings'), 'vault')

    expect(screen.getByText('Vault')).toBeInTheDocument()
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

  it('pins a page into a Pinned group and unpins it', async () => {
    const user = userEvent.setup()
    renderSidebar()

    expect(screen.queryByText('Pinned')).not.toBeInTheDocument()

    await user.click(screen.getByText('People and access'))
    const pin = screen.getByRole('button', { name: 'Pin Users' })
    expect(pin).toHaveAttribute('aria-pressed', 'false')
    await user.click(pin)

    expect(screen.getByText('Pinned')).toBeInTheDocument()
    expect(
      JSON.parse(localStorage.getItem('settings-sidebar-pins:v1') ?? '[]'),
    ).toHaveLength(1)
    const unpin = screen.getAllByRole('button', { name: 'Unpin Users' })
    expect(unpin[0]).toHaveAttribute('aria-pressed', 'true')

    await user.click(unpin[0]!)
    expect(screen.queryByText('Pinned')).not.toBeInTheDocument()
  })

  it('skips stored pins that no longer match a page', () => {
    localStorage.setItem(
      'settings-sidebar-pins:v1',
      JSON.stringify(['/settings/gone']),
    )
    renderSidebar()

    expect(screen.queryByText('Pinned')).not.toBeInTheDocument()
  })
})
