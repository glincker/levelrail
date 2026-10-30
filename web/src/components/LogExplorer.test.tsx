import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import type { ReactElement } from 'react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import observabilityEn from '../locales/en/observability.json'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { LogSignaturesPanel } from './LogSignaturesPanel'
import { SavedLogQueriesMenu } from './SavedLogQueriesMenu'
import { CreateLogAlertDialog } from './CreateLogAlertDialog'
import type { LogSignatures, SavedLogQuery } from '../types/logExplorer'
import { createTestQueryClient } from '@/test/testQueryClient'

const SIGNATURES: LogSignatures = {
  from: '2026-10-11T00:00:00Z',
  to: '2026-10-11T01:00:00Z',
  scanned: 12,
  max_rows: 20000,
  truncated: false,
  total: 2,
  new_count: 1,
  has_baseline: true,
  signatures: [
    {
      id: 'aaa111',
      template: 'db timeout after <num>ms',
      level: 'error',
      count: 5,
      first_seen: '2026-10-11T00:01:00Z',
      last_seen: '2026-10-11T00:50:00Z',
      buckets: [0, 1, 4],
      sample: 'ERROR db timeout after 301ms',
    },
    {
      id: 'bbb222',
      template: 'cannot render template <hex>',
      level: 'error',
      count: 2,
      first_seen: '2026-10-11T00:30:00Z',
      last_seen: '2026-10-11T00:40:00Z',
      buckets: [0, 0, 2],
      sample: 'ERROR cannot render template deadbeef12',
      new: true,
    },
  ],
}

const SAVED: SavedLogQuery[] = [
  {
    id: 'slq_1',
    name: 'Payment errors',
    app: 'web',
    spec: { q: 'payment', level: 'error' },
    visibility: 'instance',
    pinned: true,
    mine: true,
    created_at: '2026-10-11T00:00:00Z',
    updated_at: '2026-10-11T00:00:00Z',
  },
]

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['observability'],
  defaultNS: 'observability',
  resources: { en: { observability: observabilityEn } },
  interpolation: { escapeValue: false },
})

function wrapper(ui: ReactElement) {
  return (
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={createTestQueryClient()}>
        {ui}
      </QueryClientProvider>
    </I18nextProvider>
  )
}

interface FetchInit {
  method?: string
  body?: string
}

function stubFetch(handler: (url: string, init?: FetchInit) => unknown) {
  const fn = vi.fn((input: string, init?: FetchInit) =>
    Promise.resolve({
      ok: true,
      status: 200,
      json: () => Promise.resolve(handler(input, init)),
    } as unknown as Response),
  )
  vi.stubGlobal('fetch', fn)
  return fn
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('LogSignaturesPanel', () => {
  it('lists signatures with counts and filters to new ones', async () => {
    const fetchMock = stubFetch(() => SIGNATURES)
    render(
      wrapper(
        <LogSignaturesPanel
          appName="web"
          onViewLines={() => undefined}
          onAlert={() => undefined}
        />,
      ),
    )
    expect(
      await screen.findByText('db timeout after <num>ms'),
    ).toBeInTheDocument()
    expect(screen.getByText('5 occurrences')).toBeInTheDocument()
    expect(fetchMock.mock.calls[0]?.[0]).toContain('baseline=true')

    await userEvent.click(screen.getByRole('checkbox'))
    expect(screen.queryByText('db timeout after <num>ms')).toBeNull()
    expect(screen.getByText('cannot render template <hex>')).toBeInTheDocument()
  })

  it('reports the signature to open and to alert on', async () => {
    stubFetch(() => SIGNATURES)
    const onViewLines = vi.fn()
    const onAlert = vi.fn()
    render(
      wrapper(
        <LogSignaturesPanel
          appName="web"
          onViewLines={onViewLines}
          onAlert={onAlert}
        />,
      ),
    )
    await screen.findByText('db timeout after <num>ms')
    await userEvent.click(
      screen.getAllByRole('button', { name: 'View lines' })[0]!,
    )
    expect(onViewLines.mock.calls[0]?.[0]).toMatchObject({ id: 'aaa111' })
    await userEvent.click(
      screen.getAllByRole('button', { name: 'Alert on this' })[1]!,
    )
    expect(onAlert.mock.calls[0]?.[0]).toMatchObject({ id: 'bbb222' })
  })
})

describe('SavedLogQueriesMenu', () => {
  it('applies a saved query', async () => {
    stubFetch(() => ({ queries: SAVED }))
    const onApply = vi.fn()
    render(
      wrapper(
        <SavedLogQueriesMenu
          appName="web"
          current={{ q: 'boom', level: 'error' }}
          currentSpec={{ q: 'boom', level: 'error', range: '1h' }}
          onApply={onApply}
        />,
      ),
    )
    await userEvent.click(screen.getByRole('button', { name: /Saved/ }))
    await userEvent.click(
      await screen.findByRole('button', { name: 'Payment errors' }),
    )
    expect(onApply).toHaveBeenCalledWith({ q: 'payment', level: 'error' })
  })

  it('saves the current search', async () => {
    const fetchMock = stubFetch((_url, init) =>
      init?.method === 'POST' ? SAVED[0] : { queries: SAVED },
    )
    render(
      wrapper(
        <SavedLogQueriesMenu
          appName="web"
          current={{ q: 'boom', level: 'error' }}
          currentSpec={{ q: 'boom', level: 'error', range: '1h' }}
          onApply={() => undefined}
        />,
      ),
    )
    await userEvent.click(screen.getByRole('button', { name: /Saved/ }))
    await userEvent.type(
      await screen.findByLabelText('Name for the current search'),
      'My boom',
    )
    await userEvent.click(
      screen.getByRole('button', { name: 'Save current search' }),
    )
    const post = fetchMock.mock.calls.find((c) => c[1]?.method === 'POST')
    expect(JSON.parse(String(post?.[1]?.body))).toMatchObject({
      name: 'My boom',
      app: 'web',
      spec: { q: 'boom', level: 'error' },
      visibility: 'private',
    })
  })
})

describe('CreateLogAlertDialog', () => {
  it('posts a log_pattern rule with the search as its matcher', async () => {
    const fetchMock = stubFetch(() => ({
      id: 'r1',
      name: 'x',
      kind: 'log_pattern',
    }))
    render(
      wrapper(
        <CreateLogAlertDialog
          appName="web"
          open
          onOpenChange={() => undefined}
          defaults={{ name: 'web log alert', query: 'timeout', level: 'error' }}
        />,
      ),
    )
    await userEvent.click(screen.getByRole('button', { name: 'Create alert' }))
    const post = fetchMock.mock.calls.find((c) => c[1]?.method === 'POST')
    expect(String(post?.[0])).toBe('/api/v1/apps/web/alerts')
    expect(JSON.parse(String(post?.[1]?.body))).toMatchObject({
      kind: 'log_pattern',
      log_rule: {
        query: 'timeout',
        level: 'error',
        count: 10,
        window_seconds: 300,
      },
    })
  })
})
