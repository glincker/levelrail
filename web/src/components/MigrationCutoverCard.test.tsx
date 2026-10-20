import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { MigrationCutoverCard } from './MigrationCutoverCard'
import migrationEn from '../locales/en/migration.json'
import type { CutoverReport } from '../queries/migration'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['migration'],
  defaultNS: 'migration',
  resources: { en: { migration: migrationEn } },
  interpolation: { escapeValue: false },
})

const report: CutoverReport = {
  verdict: 'wait',
  phase: 'pre-switch',
  target_ips: ['203.0.113.10'],
  guidance: ['The source keeps serving traffic until you change DNS.'],
  domains: [
    {
      app: 'web',
      domain: 'app.example.com',
      verdict: 'wait',
      checks: [
        {
          id: 'dns-ttl',
          status: 'warn',
          detail: 'TTL is 3600s',
          fix: 'set the TTL of app.example.com to 300',
        },
      ],
      change: {
        type: 'A',
        name: 'app.example.com',
        value: '203.0.113.10',
        ttl: 300,
        replaces: '198.51.100.7',
      },
    },
  ],
}

function renderCard() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={client}>
        <MigrationCutoverCard />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('MigrationCutoverCard', () => {
  it('shows the verdict, the TTL fix and the exact record change', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: () => Promise.resolve(report),
    })
    vi.stubGlobal('fetch', fetchMock)
    renderCard()

    fireEvent.change(screen.getByLabelText("This node's public IP"), {
      target: { value: '203.0.113.10' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Run the check' }))

    await waitFor(() =>
      expect(
        screen.getByText('set the TTL of app.example.com to 300'),
      ).toBeTruthy(),
    )
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/migration/cutover?target_ip=203.0.113.10',
      undefined,
    )
    expect(
      screen.getByText('Set A app.example.com to 203.0.113.10 (TTL 300)'),
    ).toBeTruthy()
  })
})
