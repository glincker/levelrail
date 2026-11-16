import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { ErrorsPanel } from './ErrorsPanel'
import commonEn from '../../locales/en/common.json'
import errorsEn from '../../locales/en/errors.json'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@/components/ui/toast', () => ({ toast: { add: vi.fn() } }))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['errors', 'common'],
  defaultNS: 'errors',
  resources: { en: { errors: errorsEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

const issue = {
  fingerprint: 'fp1',
  title: 'TypeError: boom',
  level: 'error',
  status: 'open',
  count: 4,
  first_seen_ns: 1,
  last_seen_ns: 2,
}

function mockApi(enabled: boolean, issues: unknown[]) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      const body = url.includes('/error-tracking')
        ? { enabled, endpoint: 'https://cp.example.com/api/v1/sentry/1' }
        : issues
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }),
  )
}

function renderPanel() {
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={createTestQueryClient()}>
        <ErrorsPanel app="web" />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('ErrorsPanel states', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows only the setup card when tracking is off', async () => {
    mockApi(false, [])
    renderPanel()
    expect(await screen.findByText(/Error tracking is off/)).toBeInTheDocument()
    expect(screen.queryByText('Status')).not.toBeInTheDocument()
    expect(screen.queryByText('No errors')).not.toBeInTheDocument()
  })

  it('shows a calm empty state without filters when tracking is on and there are no issues', async () => {
    mockApi(true, [])
    renderPanel()
    expect(await screen.findByText('No errors')).toBeInTheDocument()
    expect(screen.queryByText('Status')).not.toBeInTheDocument()
    expect(screen.queryByText('Window')).not.toBeInTheDocument()
  })

  it('shows filters and issues when issues exist', async () => {
    mockApi(true, [issue])
    renderPanel()
    expect(await screen.findByText('TypeError: boom')).toBeInTheDocument()
    expect(screen.getByText('Status')).toBeInTheDocument()
    expect(screen.getByText('Window')).toBeInTheDocument()
  })
})
