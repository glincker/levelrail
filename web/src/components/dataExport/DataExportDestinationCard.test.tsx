import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import { DataExportDestinationCard } from './DataExportDestinationCard'
import dataExportEn from '../../locales/en/dataExport.json'
import type { DataExportDestination } from '../../types/dataExport'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@/components/ui/toast', () => ({
  toast: { add: vi.fn() },
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['dataExport'],
  defaultNS: 'dataExport',
  resources: { en: { dataExport: dataExportEn } },
  interpolation: { escapeValue: false },
})

const base: DataExportDestination = {
  id: 'exp_1',
  name: 'prod loki',
  kind: 'logs_loki',
  endpoint: 'https://loki.example.com:3100',
  enabled: true,
  apps: [],
  min_level: '',
  options: {},
  header_names: ['X-Scope-OrgID'],
  has_hmac_key: false,
  status: {
    last_success_at: '',
    last_error: 'loki returned status 503',
    last_error_at: '',
    sent_total: 12,
    dropped_total: 3,
    failures_total: 4,
    queue_depth: 7,
    circuit_open: true,
  },
  created_at: '',
  updated_at: '',
}

function renderCard(dest: DataExportDestination) {
  const client = createTestQueryClient()
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <DataExportDestinationCard destination={dest} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

describe('DataExportDestinationCard', () => {
  it('shows delivery state and never a secret value', () => {
    renderCard(base)
    expect(screen.getByText('prod loki')).toBeInTheDocument()
    expect(screen.getByText('Circuit open, retrying later')).toBeInTheDocument()
    expect(screen.getByText(/loki returned status 503/)).toBeInTheDocument()
    expect(screen.getByText('7')).toBeInTheDocument()
    expect(screen.getByText('Never')).toBeInTheDocument()
  })

  it('posts the test probe and reports a failure', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        new Response(JSON.stringify({ ok: false, error: 'status 503' }), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderCard(base)

    await userEvent.click(screen.getByRole('button', { name: 'Send test' }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    const calls = fetchMock.mock.calls as unknown as [string, RequestInit][]
    const first = calls[0]
    expect(first?.[0]).toBe('/api/v1/export/destinations/exp_1/test')
    expect(first?.[1].method).toBe('POST')
    await waitFor(() => {
      expect(toast.add).toHaveBeenCalledWith(
        expect.objectContaining({ type: 'error' }),
      )
    })
  })

  it('offers to accept a self-signed certificate after a TLS failure and retests', async () => {
    const responses = [
      { ok: false, error: 'x509: unknown authority', fix: 'tls_error' },
      { ...base, options: { insecure_skip_verify: true } },
      { ok: true },
    ]
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        new Response(JSON.stringify(responses.shift()), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderCard({
      ...base,
      kind: 'audit_splunk',
      options: { index: 'audit' },
      status: { ...base.status, circuit_open: false },
    })

    await userEvent.click(screen.getByRole('button', { name: 'Send test' }))
    await userEvent.click(
      await screen.findByRole('button', {
        name: 'Accept this certificate and test again',
      }),
    )

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(3)
    })
    const calls = fetchMock.mock.calls as unknown as [string, RequestInit][]
    expect(calls[1]?.[1].method).toBe('PATCH')
    expect(JSON.parse(calls[1]?.[1].body as string)).toEqual({
      options: { index: 'audit', insecure_skip_verify: true },
    })
    expect(calls[2]?.[0]).toBe('/api/v1/export/destinations/exp_1/test')
  })
})
