import type { AnchorHTMLAttributes, ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { StatusStrip } from './StatusStrip'
import dashboardEn from '../../locales/en/dashboard.json'
import commonEn from '../../locales/en/common.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['dashboard', 'common'],
  defaultNS: 'dashboard',
  fallbackNS: 'common',
  resources: { en: { dashboard: dashboardEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Link: ({
      children,
      to,
      params,
      ...rest
    }: {
      children?: ReactNode
      to?: string
      params?: { id?: string }
    } & AnchorHTMLAttributes<HTMLAnchorElement>) => (
      <a href={to?.replace('$id', params?.id ?? '')} {...rest}>
        {children}
      </a>
    ),
  }
})
vi.mock('../../hooks/useIsRoot', () => ({ useIsRoot: () => false }))
vi.mock('./useDashboardAttention', () => ({
  useDashboardAttention: () => ({
    level: 'ok',
    count: 0,
    visible: [],
    snoozed: [],
    isLoading: false,
  }),
}))
vi.mock('../../hooks/useDashboardNotices', () => ({
  useDashboardNotices: () => [],
}))

function renderStrip(nodes: unknown[]) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(nodes),
        text: () => Promise.resolve('[]'),
      }),
    ),
  )
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <StatusStrip apps={3} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('StatusStrip offline nodes', () => {
  it('names the offline node and links to it', async () => {
    renderStrip([
      {
        id: 'n1',
        name: 'edge-1',
        status: 'offline',
        offline_since: '2026-10-25T00:00:00Z',
      },
      { id: 'n2', name: 'edge-2', status: 'online' },
    ])
    const link = await screen.findByRole('link', { name: /edge-1 is offline/ })
    expect(link.getAttribute('href')).toBe('/nodes/n1')
    expect(screen.queryByText(/apps healthy/)).toBeNull()
  })

  it('keeps the healthy line when every node is online', async () => {
    renderStrip([{ id: 'n2', name: 'edge-2', status: 'online' }])
    expect(await screen.findByText(/All 3 apps healthy/)).toBeTruthy()
  })
})
