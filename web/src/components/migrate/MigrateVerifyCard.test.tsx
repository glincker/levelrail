import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AllI18n } from '../../test/allI18n'
import { MigrateVerifyCard } from './MigrateVerifyCard'
import { createTestQueryClient } from '@/test/testQueryClient'

const report = {
  platform: 'coolify',
  generated_at: '2026-10-22T10:00:00Z',
  verdict: 'fail',
  counts: { blocking: 1, warning: 0, info: 1 },
  categories: [
    {
      category: 'env',
      source: 2,
      target: 1,
      matched: 1,
      blocking: 1,
      warning: 0,
      info: 0,
    },
  ],
  differences: [
    {
      category: 'env',
      severity: 'blocking',
      app: 'web',
      subject: 'DB_URL',
      detail: 'variable name missing here',
      fix: 'set DB_URL on the app',
    },
    {
      category: 'apps',
      severity: 'info',
      subject: 'stray',
      detail: 'exists here but not on the source',
      fix: 'nothing to do',
    },
  ],
  probes: [
    {
      app: 'web',
      domain: 'web.example.com',
      source: {
        url: 'https://web.example.com/',
        status: 200,
        latency_ms: 40,
        tls: true,
        tls_valid: true,
        tls_days_left: 60,
      },
      target: {
        url: 'https://web.example.com/',
        error: 'refused',
        tls: false,
        tls_valid: false,
      },
    },
  ],
  can_recheck: true,
}

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    }),
  )
}

describe('MigrateVerifyCard', () => {
  const calls: { method: string; url: string; body?: string }[] = []
  let latest: () => Promise<Response>
  let verify: () => Promise<Response>

  beforeEach(() => {
    calls.length = 0
    latest = () => json(report)
    verify = () =>
      json({
        ...report,
        verdict: 'pass',
        counts: { blocking: 0, warning: 0, info: 0 },
        differences: null,
      })
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push({
          method: init?.method ?? 'GET',
          url,
          body: init?.body as string | undefined,
        })
        if (url === '/api/v1/migrations/verify/latest') return latest()
        if (url === '/api/v1/migrations/verify') return verify()
        return json({ error: 'unexpected ' + url }, 500)
      }),
    )
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  function renderCard() {
    const qc = createTestQueryClient()
    render(
      <QueryClientProvider client={qc}>
        <AllI18n>
          <MigrateVerifyCard />
        </AllI18n>
      </QueryClientProvider>,
    )
  }

  it('shows the verdict, counts, differences with their fix and the probes', async () => {
    renderCard()
    expect(await screen.findByText('Blocking differences')).toBeInTheDocument()
    expect(screen.getByText('1 Blocking')).toBeInTheDocument()
    expect(screen.getAllByText(/DB_URL/).length).toBeGreaterThan(0)
    expect(screen.getByText('Fix: set DB_URL on the app')).toBeInTheDocument()
    expect(screen.getByText('web.example.com')).toBeInTheDocument()
    expect(screen.getByText(/No answer: refused/)).toBeInTheDocument()
  })

  it('re-checks with the held credential and replaces the report', async () => {
    const user = userEvent.setup()
    renderCard()
    await user.click(await screen.findByRole('button', { name: 'Re-check' }))
    await waitFor(() => expect(screen.getByText('Matches')).toBeInTheDocument())
    const post = calls.find((c) => c.method === 'POST')
    expect(post?.url).toBe('/api/v1/migrations/verify')
    expect(JSON.parse(post?.body ?? '{}')).toEqual({ recheck: true })
    expect(screen.getByText('Nothing differs.')).toBeInTheDocument()
  })

  it('asks for the source token again when the held one expired', async () => {
    const user = userEvent.setup()
    verify = () => json({ error: 'expired' }, 409)
    renderCard()
    await user.click(await screen.findByRole('button', { name: 'Re-check' }))
    expect(
      await screen.findByText(
        'The source token expired. Enter it again to re-check.',
      ),
    ).toBeInTheDocument()
    expect(screen.getByLabelText('API token')).toBeInTheDocument()
  })

  it('offers the form when no check has run and never keeps the token in the DOM after a run', async () => {
    const user = userEvent.setup()
    latest = () => json({ error: 'none' }, 404)
    renderCard()
    expect(await screen.findByText('No check run yet')).toBeInTheDocument()
    await user.type(
      screen.getByLabelText('Source URL'),
      'https://coolify.example.com',
    )
    await user.type(screen.getByLabelText('API token'), 'src-token')
    await user.click(screen.getByRole('button', { name: 'Check parity' }))
    await waitFor(() => expect(screen.getByText('Matches')).toBeInTheDocument())
    const post = calls.find((c) => c.method === 'POST')
    expect(JSON.parse(post?.body ?? '{}')).toMatchObject({
      platform: 'coolify',
      url: 'https://coolify.example.com',
      token: 'src-token',
    })
  })
})
