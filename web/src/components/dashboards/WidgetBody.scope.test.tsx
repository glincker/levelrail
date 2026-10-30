import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { WidgetBody } from './WidgetBody'
import { resetProjectScopeForTests, setProjectScope } from '@/lib/projectScope'
import { resolveTimeRange } from '@/lib/timeRange'
import dashboardsEn from '@/locales/en/dashboards.json'
import dashboardEn from '@/locales/en/dashboard.json'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['dashboards', 'dashboard'],
  defaultNS: 'dashboards',
  resources: { en: { dashboards: dashboardsEn, dashboard: dashboardEn } },
  interpolation: { escapeValue: false },
})

const json = (body: unknown) =>
  Promise.resolve(new Response(JSON.stringify(body), { status: 200 }))

function mockApi(urls: string[]) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      urls.push(url)
      if (url.startsWith('/api/v1/resources')) {
        return json({ items: [], total: 0, counts: { app: 9, database: 9 } })
      }
      if (url.startsWith('/api/v1/apps-summary')) return json({ total: 2 })
      if (url.startsWith('/api/v1/databases')) return json([{ name: 'a' }])
      return json({})
    }),
  )
}

function renderWidget() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <WidgetBody
          w={{ id: 'c', type: 'resource_counts', size: 'L' }}
          range={resolveTimeRange('24h')}
        />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  resetProjectScopeForTests()
})

describe('resource counts widget scope', () => {
  it('uses the scoped app and database lists when a project is picked', async () => {
    const urls: string[] = []
    mockApi(urls)
    setProjectScope('p1')
    renderWidget()
    await waitFor(() => {
      expect(screen.getByText('2')).toBeTruthy()
    })
    expect(urls).toContain('/api/v1/apps-summary?project=p1')
    expect(urls).toContain('/api/v1/databases?project=p1')
    expect(screen.queryByText('9')).toBeNull()
  })

  it('labels nodes and domains as instance-wide only while scoped', async () => {
    const urls: string[] = []
    mockApi(urls)
    setProjectScope('p1')
    renderWidget()
    await waitFor(() => {
      expect(screen.getAllByText('All projects')).toHaveLength(2)
    })
  })

  it('shows no instance-wide label without a scope', async () => {
    const urls: string[] = []
    mockApi(urls)
    renderWidget()
    await waitFor(() => {
      expect(screen.getAllByText('9')).toHaveLength(2)
    })
    expect(screen.queryByText('All projects')).toBeNull()
  })

  it('keeps the instance-wide counts without a scope', async () => {
    const urls: string[] = []
    mockApi(urls)
    renderWidget()
    await waitFor(() => {
      expect(screen.getAllByText('9')).toHaveLength(2)
    })
    expect(urls.some((u) => u.startsWith('/api/v1/apps-summary'))).toBe(false)
  })
})
