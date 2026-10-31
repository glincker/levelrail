import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PortTakeoverPanel } from './PortTakeoverPanel'
import commonEn from '../locales/en/common.json'
import migrationEn from '../locales/en/migration.json'
import type {
  TakeoverPlan,
  TakeoverRun,
  TakeoverStatus,
} from '../queries/portTakeover'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('../hooks/useIsRoot', () => ({ useIsRoot: () => true }))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['migration', 'common'],
  defaultNS: 'migration',
  resources: { en: { migration: migrationEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

function plan(verdict: TakeoverPlan['verdict']): TakeoverPlan {
  return {
    verdict,
    checks: [
      {
        id: 'ports',
        title: 'Ports 80 and 443 are held by a proxy this run can stop',
        status: verdict === 'blocked' ? 'block' : 'pass',
        detail: 'A host service holds the port.',
        fix:
          verdict === 'blocked'
            ? {
                summary: 'Stop that service, then re-check.',
                action: {
                  kind: 'copy',
                  label: 'Copy command',
                  value: 'sudo ss -ltnp',
                },
              }
            : undefined,
      },
    ],
    steps: [
      {
        id: 'stop_old_proxy',
        title: 'Stop the old proxy',
        detail: 'Stop coolify-proxy.',
        reversible: true,
      },
    ],
    domains: [
      {
        domain: 'shop.example.com',
        app: 'shop',
        healthy: true,
        dns: 'ok',
        certificate: 'acme',
      },
    ],
    rollback: ['Start coolify-proxy again.'],
    window_seconds: 300,
    target_http: ':80',
    target_https: ':443',
    current_http: ':8088',
    current_https: ':8443',
    proxy_containers: ['coolify-proxy'],
    checked_at: '2026-10-20T00:00:00Z',
  }
}

const doneRun: TakeoverRun = {
  id: 'tk-1',
  state: 'done',
  steps: [
    { id: 'stop_old_proxy', title: 'Stop', state: 'done', detail: 'stopped' },
  ],
  verify: [],
  window_seconds: 300,
  created_at: '2026-10-20T00:00:00Z',
  updated_at: '2026-10-20T00:01:00Z',
  rollbackable: true,
  in_flight: false,
}

function mockFetch(status: TakeoverStatus) {
  const calls: { url: string; init?: RequestInit }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      calls.push({ url, init })
      const body = url.includes('/runs')
        ? { created: true, run: doneRun }
        : url.includes('/system/restart')
          ? { boot_id: 'b1', supported: true, pending: false }
          : status
      return Promise.resolve(
        new Response(JSON.stringify(body), { status: 200 }),
      )
    }),
  )
  return calls
}

function renderPanel() {
  const qc = createTestQueryClient()
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={testI18n}>
        <PortTakeoverPanel />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('PortTakeoverPanel', () => {
  it('shows a blocking check with its fix and keeps Run off', async () => {
    mockFetch({ plan: plan('blocked'), run: null })
    renderPanel()
    expect((await screen.findAllByText('Blocked')).length).toBe(2)
    expect(screen.getByText('Stop that service, then re-check.')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Copy command' })).toBeTruthy()
    const run = screen.getByRole('button', { name: 'Take over ports' })
    expect((run as HTMLButtonElement).disabled).toBe(true)
  })

  it('lists the steps and rollback of a ready plan', async () => {
    mockFetch({ plan: plan('ready'), run: null })
    renderPanel()
    expect(await screen.findByText('Stop the old proxy')).toBeTruthy()
    expect(screen.getByText('Start coolify-proxy again.')).toBeTruthy()
    expect(screen.getByText('shop.example.com')).toBeTruthy()
  })

  it('needs the typed word before it starts a run', async () => {
    const calls = mockFetch({ plan: plan('ready'), run: null })
    const user = userEvent.setup()
    renderPanel()
    await user.click(
      await screen.findByRole('button', { name: 'Take over ports' }),
    )
    const confirm = await screen.findByRole('button', { name: 'Take over' })
    expect((confirm as HTMLButtonElement).disabled).toBe(true)
    await user.type(screen.getByRole('textbox'), 'takeover')
    await user.click(confirm)
    await waitFor(() =>
      expect(calls.some((c) => c.init?.method === 'POST')).toBe(true),
    )
    const post = calls.find((c) => c.init?.method === 'POST')
    expect(JSON.parse(post?.init?.body as string)).toEqual({
      confirm: true,
      accept_unmanaged: false,
    })
  })

  it('offers rollback for a finished run', async () => {
    mockFetch({ plan: plan('done'), run: doneRun })
    renderPanel()
    expect(
      await screen.findByRole('button', { name: 'Roll back' }),
    ).toBeTruthy()
    expect(screen.getByText(/already listens on ports 80 and 443/)).toBeTruthy()
  })
})
