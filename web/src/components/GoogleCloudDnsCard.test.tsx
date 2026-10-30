import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { GoogleCloudDnsCard } from './GoogleCloudDnsCard'
import domainsEn from '../locales/en/domains.json'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@/components/ui/toast', () => ({
  toast: { add: vi.fn() },
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

const saved = {
  enabled: true,
  project_id: 'proj-1',
  client_email: 'sa@proj-1.iam.gserviceaccount.com',
  has_key: true,
}

function stubApi(testBody: unknown) {
  const calls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      calls.push(`${init?.method ?? 'GET'} ${url}`)
      const body = url.endsWith('/test') ? testBody : saved
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }),
  )
  return calls
}

function renderCard() {
  const client = createTestQueryClient()
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <GoogleCloudDnsCard />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('GoogleCloudDnsCard', () => {
  it('shows only the project and client email, and Test lists zones', async () => {
    const calls = stubApi({ ok: true, zones: ['example.com'] })
    renderCard()
    expect(await screen.findByText('proj-1')).toBeInTheDocument()
    expect(
      screen.getByText('sa@proj-1.iam.gserviceaccount.com'),
    ).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: 'Test' }))
    expect(
      await screen.findByText(/Found 1 managed zone\(s\): example\.com/),
    ).toBeInTheDocument()
    await waitFor(() => {
      expect(calls).toContain('POST /api/v1/settings/google-cloud-dns/test')
    })
  })

  it('reports a failed test', async () => {
    stubApi({ ok: false, error: 'permission denied' })
    renderCard()
    await userEvent.click(await screen.findByRole('button', { name: 'Test' }))
    expect(
      await screen.findByText('Could not list zones: permission denied'),
    ).toBeInTheDocument()
  })
})
