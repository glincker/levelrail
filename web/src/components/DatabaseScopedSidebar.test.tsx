import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import en from '../locales/en/databases.json'
import { DatabaseScopedSidebar } from './DatabaseScopedSidebar'
import { SidebarProvider } from '@/components/ui/sidebar'

// The active nav entry's highlight was purely visual (a `data-active`
// CSS hook, see ui/sidebar.tsx's SidebarMenuButton) with nothing telling
// assistive tech which page is current. This locks in aria-current
// instead of only checking the visible highlight class.

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    // Forwards every prop (aria-current included) onto the rendered <a>,
    // unlike a children/to-only stub: SidebarMenuButton passes aria-current
    // through its `render` element, and a stub that drops unknown props
    // would silently hide that from this test.
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
    useRouterState: () => '/databases/main/metrics',
  }
})

vi.mock('../queries/databaseViewer', () => ({
  useExplorerCapabilities: () => ({ data: undefined }),
}))

vi.mock('../queries/databases', () => ({
  useDatabase: () => ({ data: { name: 'main', engine: 'postgres' } }),
  useDatabaseStatus: () => ({ data: [] }),
}))

describe('DatabaseScopedSidebar active nav state', () => {
  beforeEach(async () => {
    await i18n.use(initReactI18next).init({
      lng: 'en',
      resources: { en: { databases: en } },
      defaultNS: 'databases',
    })
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
  })

  it('marks the current section with aria-current, not just a visual highlight', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <SidebarProvider>
          <DatabaseScopedSidebar name="main" />
        </SidebarProvider>
      </I18nextProvider>,
    )

    expect(
      screen.getByRole('link', { name: /Monitoring/, current: 'page' }),
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Overview/ })).not.toHaveAttribute(
      'aria-current',
    )
  })
})
