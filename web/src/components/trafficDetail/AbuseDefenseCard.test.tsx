import { QueryClientProvider } from '@tanstack/react-query'
import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import trafficEn from '../../locales/en/trafficDetail.json'
import type {
  AbuseCounters,
  AbuseDryRun,
  AbusePolicy,
  AppAbuse,
} from '../../types/abuse'
import { AbuseDefenseCard } from './AbuseDefenseCard'
import { draftFrom, draftToPolicy, parseList } from './abuseDraft'
import { createTestQueryClient } from '@/test/testQueryClient'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['trafficDetail'],
  defaultNS: 'trafficDetail',
  resources: { en: { trafficDetail: trafficEn } },
  interpolation: { escapeValue: false },
})

const counters: AbuseCounters = {
  allowed: 120,
  would_block: 7,
  blocked: 0,
  fail_open: 0,
  by_reason: [],
}

function abuse(over: Partial<AppAbuse> = {}): AppAbuse {
  return {
    app: 'web',
    configured: false,
    policy: { mode: 'off' },
    condition: { status: 'False', reason: 'Off', message: 'off' },
    suggestion: { peak_rps: 80, rps: 20, burst: 40, from_traffic: true },
    counters,
    counters_window_minutes: 15,
    limits: {
      max_rps: 10000,
      max_burst: 100000,
      max_deny_entries: 256,
      max_agent_entries: 64,
      dry_run_default_minutes: 15,
      dry_run_max_minutes: 60,
    },
    ...over,
  }
}

const dryRun: AbuseDryRun = {
  requests: 400,
  would_block: 30,
  percent: 7.5,
  by_reason: [{ reason: 'rate_limit', count: 30 }],
  top_clients: [
    {
      client: '192.0.2.50',
      requests: 90,
      would_block: 30,
      top_reason: 'rate_limit',
    },
  ],
  window_minutes: 15,
  covered_minutes: 9.4,
  note: 'Preview only.',
}

interface Calls {
  puts: AbusePolicy[]
  previews: AbusePolicy[]
}

function mockApi(get: AppAbuse): Calls {
  const calls: Calls = { puts: [], previews: [] }
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      const raw = typeof init?.body === 'string' ? init.body : ''
      const body = raw ? (JSON.parse(raw) as unknown) : null
      if (url.endsWith('/dry-run')) {
        calls.previews.push((body as { policy: AbusePolicy }).policy)
        return Promise.resolve(Response.json(dryRun))
      }
      if (init?.method === 'PUT') {
        const p = body as AbusePolicy
        calls.puts.push(p)
        return Promise.resolve(
          Response.json({ ...get, configured: true, policy: p }),
        )
      }
      return Promise.resolve(Response.json(get))
    }),
  )
  return calls
}

function renderCard() {
  const qc = createTestQueryClient()
  return render(
    <QueryClientProvider client={qc}>
      <AbuseDefenseCard app="web" counters={counters} />
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  vi.useRealTimers()
})

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

describe('abuseDraft', () => {
  it('prefills an unconfigured app in log_only from the suggestion', () => {
    const d = draftFrom(abuse())
    expect(d).toMatchObject({ mode: 'log_only', rps: '20', burst: '40' })
    expect(draftToPolicy(d)).toEqual({ mode: 'log_only', rps: 20, burst: 40 })
  })

  it('keeps a saved policy as it is', () => {
    const d = draftFrom(
      abuse({
        configured: true,
        policy: {
          mode: 'enforce',
          rps: 5,
          burst: 9,
          deny_cidrs: ['10.0.0.0/8'],
          require_user_agent: true,
        },
      }),
    )
    expect(draftToPolicy(d)).toEqual({
      mode: 'enforce',
      rps: 5,
      burst: 9,
      deny_cidrs: ['10.0.0.0/8'],
      require_user_agent: true,
    })
  })

  it('splits lists on lines and commas and rejects a non number', () => {
    expect(parseList('a, b\n\n c ')).toEqual(['a', 'b', 'c'])
    const d = draftFrom(abuse())
    expect(draftToPolicy({ ...d, rps: 'fast' })).toBeNull()
    expect(draftToPolicy({ ...d, rps: '0' })).toEqual({ mode: 'log_only' })
  })
})

describe('AbuseDefenseCard', () => {
  it('never shows a blank form: rate and burst come from the suggestion', async () => {
    mockApi(abuse())
    renderCard()
    expect(await screen.findByDisplayValue('20')).toBeTruthy()
    expect(screen.getByDisplayValue('40')).toBeTruthy()
    expect(screen.getByText(/Suggested from your peak of 80/)).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Log only' }).ariaPressed).toBe(
      'true',
    )
  })

  it('shows what the draft would have blocked and offers log only first', async () => {
    const calls = mockApi(abuse())
    const user = userEvent.setup()
    renderCard()
    expect(
      await screen.findByText(/Would have blocked 30 of 400 recent requests/),
    ).toBeTruthy()
    expect(screen.getByText(/192\.0\.2\.50 \(30\)/)).toBeTruthy()
    expect(calls.previews[0]).toMatchObject({ mode: 'enforce', rps: 20 })
    const save = screen.getByRole('button', { name: 'Start in log only' })
    await user.click(save)
    await waitFor(() => {
      expect(calls.puts).toEqual([{ mode: 'log_only', rps: 20, burst: 40 }])
    })
  })

  it('offers enforcing only for a saved log only policy, after the preview', async () => {
    const calls = mockApi(
      abuse({
        configured: true,
        policy: { mode: 'log_only', rps: 20, burst: 40 },
        condition: { status: 'True', reason: 'LogOnly', message: 'counting' },
      }),
    )
    const user = userEvent.setup()
    renderCard()
    expect(await screen.findByText('Counting only')).toBeTruthy()
    const go = await screen.findByRole('button', { name: 'Start enforcing' })
    await waitFor(() => {
      expect((go as HTMLButtonElement).disabled).toBe(false)
    })
    expect(calls.previews.length).toBeGreaterThan(0)
    await user.click(go)
    await waitFor(() => {
      expect(calls.puts).toEqual([{ mode: 'enforce', rps: 20, burst: 40 }])
    })
  })

  it('keeps Enforce locked until the preview has loaded', async () => {
    mockApi(abuse({ configured: false }))
    // A preview that never answers keeps the lock on.
    const original = globalThis.fetch
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) =>
        url.endsWith('/dry-run')
          ? new Promise<Response>(() => undefined)
          : original(url, init),
      ),
    )
    renderCard()
    const enforce = await screen.findByRole('button', { name: 'Enforce' })
    expect((enforce as HTMLButtonElement).disabled).toBe(true)
    expect(screen.getByText(/Enforcing unlocks once the preview/)).toBeTruthy()
  })

  it('explains a hidden client address', async () => {
    mockApi(
      abuse({
        configured: true,
        policy: { mode: 'enforce', rps: 5, burst: 9 },
        condition: { status: 'False', reason: 'ClientIPHidden', message: 'x' },
      }),
    )
    renderCard()
    expect(await screen.findByText('Client addresses hidden')).toBeTruthy()
    expect(screen.getByText(/every visitor would share one limit/)).toBeTruthy()
  })
})
