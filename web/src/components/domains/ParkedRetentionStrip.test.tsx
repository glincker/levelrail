import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ParkedDomain } from '../../queries/parkedDomains'
import type { ParkedRetention } from '../../queries/parkedRetention'
import { ParkedRetentionStrip } from './ParkedRetentionStrip'
import domainsEn from '../../locales/en/domains.json'
import commonEn from '../../locales/en/common.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['domains', 'common'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

const old = new Date(Date.now() - 100 * 86_400_000).toISOString()
const parked: ParkedDomain[] = [
  { domain: 'old.com', kind: 'parked', parked_at: old },
]

function stub(status: number, body: ParkedRetention | null) {
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    void init
    return Promise.resolve({
      ok: status < 400,
      status,
      json: () => Promise.resolve(url.includes('retention') ? body : {}),
      text: () => Promise.resolve(''),
      clone() {
        return this
      },
    } as unknown as Response)
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function renderStrip() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={testI18n}>
        <ParkedRetentionStrip parked={parked} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('ParkedRetentionStrip', () => {
  it('says cleanup is off and offers preview and turn on', async () => {
    stub(200, {
      retention_days: 0,
      would_delete: [],
      total_eligible: 0,
      batch_limit: 50,
    })
    renderStrip()
    expect(await screen.findByText(/^Off\./)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Preview' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Turn on' })).toBeInTheDocument()
  })

  it('shows days, due count and a turn off action when on', async () => {
    stub(200, {
      retention_days: 30,
      would_delete: [],
      total_eligible: 2,
      batch_limit: 50,
      next_run: '2026-11-02T10:00:00Z',
    })
    renderStrip()
    expect(await screen.findByText(/after 30 days/)).toBeInTheDocument()
    expect(screen.getByText(/Due now: 2/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Turn off' })).toBeInTheDocument()
  })

  it('hides on 403', async () => {
    const fetchMock = stub(403, null)
    const { container } = renderStrip()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    expect(container).toBeEmptyDOMElement()
  })

  it('labels the preview as an estimate and confirms what stays untouched', async () => {
    stub(200, {
      retention_days: 0,
      would_delete: [],
      total_eligible: 0,
      batch_limit: 50,
    })
    renderStrip()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Preview' }),
    )
    expect(
      screen.getByText(/^Estimate from the parked list/),
    ).toBeInTheDocument()
    expect(screen.getByText('old.com')).toBeInTheDocument()
    await userEvent.click(
      screen.getByRole('button', { name: 'Turn on at 30 days' }),
    )
    expect(
      screen.getByText(/DNS records and certificates are not touched/),
    ).toBeInTheDocument()
    expect(screen.getByText(/At most 50 domains/)).toBeInTheDocument()
  })
})
