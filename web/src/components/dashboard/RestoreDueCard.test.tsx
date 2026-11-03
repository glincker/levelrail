import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { RestoreDueCard } from './RestoreDueCard'
import dashboardEn from '../../locales/en/dashboard.json'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['dashboard'],
  defaultNS: 'dashboard',
  resources: { en: { dashboard: dashboardEn } },
  interpolation: { escapeValue: false },
})

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
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

function mockOnboarding(restoreDue: boolean) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve(
        new Response(
          JSON.stringify({
            enabled: true,
            state: 'completed',
            current_step: 'done',
            flow_version: 1,
            restore_due: restoreDue,
          }),
          { status: 200 },
        ),
      ),
    ),
  )
}

function renderCard() {
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={createTestQueryClient()}>
        <RestoreDueCard />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('RestoreDueCard', () => {
  beforeEach(() => {
    window.localStorage.clear()
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('is hidden when no restore is due', async () => {
    mockOnboarding(false)
    renderCard()
    await vi.waitFor(() => {
      expect(fetch).toHaveBeenCalled()
    })
    expect(screen.queryByText('Test a restore')).not.toBeInTheDocument()
  })

  it('shows when due and hides after dismiss', async () => {
    mockOnboarding(true)
    renderCard()
    expect(await screen.findByText(/no restore has been tested/)).toBeVisible()
    expect(
      screen.getByRole('link', { name: 'Test a restore' }),
    ).toHaveAttribute('href', '/settings/control-plane-backup')
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }))
    expect(screen.queryByText(/no restore has been tested/)).toBeNull()
  })
})
