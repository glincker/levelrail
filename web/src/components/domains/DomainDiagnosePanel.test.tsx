import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import domainsEn from '../../locales/en/domains.json'
import type {
  DiagnoseCheck,
  DiagnoseReport,
} from '../../queries/domainDiagnose'
import { parseDiagnoseChunk } from '../../queries/domainDiagnose'
import { DomainDiagnosePanel } from './DomainDiagnosePanel'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

const resolveFail: DiagnoseCheck = {
  id: 'dns.resolves',
  title: 'shop.example.com does not resolve',
  state: 'fail',
  tier: 1,
  fix: {
    summary: 'Add a CNAME record.',
    kind: 'auto',
    records: [
      { name: 'shop.example.com', type: 'CNAME', value: 'lr.example.net' },
    ],
    action: {
      kind: 'request',
      label: 'Create the record',
      api: 'POST /api/v1/apps/web/domains/shop.example.com/dns-records',
      body: { name: 'shop', type: 'CNAME', value: 'lr.example.net' },
    },
    cli: 'levelrail-cli domains diagnose shop.example.com --fix',
  },
}
const guidedCaa: DiagnoseCheck = {
  id: 'caa.blocks_issuer',
  title: "CAA records block Let's Encrypt",
  state: 'warn',
  tier: 2,
  fix: {
    summary: 'Add a CAA record.',
    kind: 'guided',
    records: [
      { name: 'example.com', type: 'CAA', value: '0 issue "letsencrypt.org"' },
    ],
  },
}
const passed: DiagnoseCheck = {
  id: 'net.port80',
  title: 'Port 80 answers',
  state: 'pass',
  tier: 2,
}

function report(over: Partial<DiagnoseReport> = {}): DiagnoseReport {
  return {
    domain: 'shop.example.com',
    checked_at: '2026-10-20T10:00:00Z',
    status: 'problems',
    probed: false,
    checks: [passed, guidedCaa, resolveFail],
    propagation: {
      state: 'waiting',
      detail: 'no resolver knows this name yet',
      next_check_seconds: 30,
    },
    ...over,
  }
}

function sse(checks: DiagnoseCheck[], rep: DiagnoseReport): Response {
  const enc = new TextEncoder()
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const check of checks) {
        controller.enqueue(
          enc.encode(`data: ${JSON.stringify({ type: 'check', check })}\n\n`),
        )
      }
      controller.enqueue(
        enc.encode(
          `data: ${JSON.stringify({ type: 'done', report: rep })}\n\n`,
        ),
      )
      controller.close()
    },
  })
  return new Response(body, { status: 200 })
}

function renderPanel() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <DomainDiagnosePanel domain="shop.example.com" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('parseDiagnoseChunk', () => {
  it('returns whole messages and keeps the unfinished tail', () => {
    const a = JSON.stringify({ type: 'check', check: passed })
    const out = parseDiagnoseChunk(`data: ${a}\n\ndata: {"type":"do`)
    expect(out.messages).toHaveLength(1)
    expect(out.rest).toBe('data: {"type":"do')
  })

  it('skips a malformed message without throwing', () => {
    const out = parseDiagnoseChunk('data: {nope\n\n')
    expect(out.messages).toEqual([])
  })
})

describe('DomainDiagnosePanel', () => {
  it('lists failures first with the record, a primary button and a collapsed CLI fix', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(sse([resolveFail, guidedCaa, passed], report()))
    vi.stubGlobal('fetch', fetchMock)
    renderPanel()

    expect(
      await screen.findByText('shop.example.com does not resolve'),
    ).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/domains/shop.example.com/diagnose/stream',
      expect.objectContaining({ headers: { Accept: 'text/event-stream' } }),
    )
    const titles = screen
      .getAllByRole('listitem')
      .map((li) => li.textContent ?? '')
    expect(
      titles.findIndex((t) => t.includes('does not resolve')),
    ).toBeLessThan(titles.findIndex((t) => t.includes('CAA records block')))
    expect(screen.getByText('lr.example.net')).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Create the record' }),
    ).toBeEnabled()
    expect(screen.getByText('Waiting for DNS')).toBeInTheDocument()
    expect(screen.getByText(/Checking again in about 30s/)).toBeInTheDocument()
    expect(screen.queryByText(/diagnose shop.example.com --fix/)).toBeNull()
    await userEvent.click(screen.getByText('Fix via CLI'))
    expect(
      await screen.findByText(
        'levelrail-cli domains diagnose shop.example.com --fix',
      ),
    ).toBeInTheDocument()
    expect(screen.queryByText('Port 80 answers')).toBeNull()
    await userEvent.click(
      screen.getByRole('button', { name: 'Show 1 other checks' }),
    )
    expect(screen.getByText('Port 80 answers')).toBeInTheDocument()
  })

  it('creates the record then re-checks', async () => {
    const fetchMock = vi.fn((url: string) => {
      if (url.endsWith('/diagnose/stream')) {
        return Promise.resolve(
          sse([resolveFail], report({ checks: [resolveFail] })),
        )
      }
      return Promise.resolve(new Response('{}', { status: 200 }))
    })
    vi.stubGlobal('fetch', fetchMock)
    renderPanel()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Create the record' }),
    )
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/apps/web/domains/shop.example.com/dns-records',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            name: 'shop',
            type: 'CNAME',
            value: 'lr.example.net',
          }),
        }),
      )
    })
    await waitFor(() => {
      const streams = fetchMock.mock.calls.filter(([u]) =>
        u.endsWith('/diagnose/stream'),
      )
      expect(streams.length).toBeGreaterThanOrEqual(2)
    })
  })

  it('shows a guided fix with its record and Re-check but no apply button', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          sse([guidedCaa], report({ checks: [guidedCaa], status: 'warnings' })),
        ),
    )
    renderPanel()
    expect(
      await screen.findByText('0 issue "letsencrypt.org"'),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: /create the record/i }),
    ).toBeNull()
    expect(
      screen.getAllByRole('button', { name: 'Re-check' }).length,
    ).toBeGreaterThan(0)
  })

  it('says everything is fine when nothing needs a fix', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        sse(
          [passed],
          report({
            checks: [passed],
            status: 'healthy',
            propagation: { state: 'settled', detail: 'every resolver agrees' },
          }),
        ),
      ),
    )
    renderPanel()
    expect(await screen.findByText('Nothing to fix.')).toBeInTheDocument()
    expect(screen.getByText('Everything checks out.')).toBeInTheDocument()
    expect(screen.getByText('DNS settled')).toBeInTheDocument()
  })

  it('explains an unattached domain with a retry', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(
          new Response('{"error":"domain not found"}', { status: 404 }),
        ),
    )
    renderPanel()
    expect(
      await screen.findByText(/not attached to an app yet/),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Try again' }),
    ).toBeInTheDocument()
  })

  it('shows the server error text when the diagnosis fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockResolvedValue(new Response('{"error":"boom"}', { status: 500 })),
    )
    renderPanel()
    expect(
      await screen.findByText(/Could not run the diagnosis: boom/),
    ).toBeInTheDocument()
  })
})
