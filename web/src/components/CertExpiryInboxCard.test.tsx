import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CertExpiryInboxCard } from './CertExpiryInboxCard'
import { certNoticeFromItem, parseCertNotice } from './certNotice'
import { AllI18n } from '../test/allI18n'

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  if (input instanceof URL) return input.toString()
  return input.url
}

function json(body: unknown, status = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function stubApi(opts: {
  channels: unknown[]
  email: Record<string, unknown>
  renewCalls?: string[]
}) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = urlOf(input)
      if (url.includes('/cert/renew')) {
        opts.renewCalls?.push(`${init?.method ?? 'GET'} ${url}`)
        return Promise.resolve(
          json(
            {
              domain: 'app.example.com',
              had_stored_certificate: true,
              status: 'accepted',
            },
            202,
          ),
        )
      }
      if (url.includes('cert-expiry/email'))
        return Promise.resolve(json(opts.email))
      if (url.includes('notification-channels'))
        return Promise.resolve(json(opts.channels))
      if (url.includes('/api/v1/certificates')) {
        return Promise.resolve(
          json([{ domain: 'app.example.com', apps: ['web'] }]),
        )
      }
      return Promise.resolve(json({}, 404))
    }),
  )
}

function renderCard(reason: string, dns?: boolean) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  render(
    <AllI18n>
      <QueryClientProvider client={queryClient}>
        <CertExpiryInboxCard
          notice={{
            domain: 'app.example.com',
            daysLeft: 2,
            issuer: 'R3',
            reason,
            dns,
          }}
        />
      </QueryClientProvider>
    </AllI18n>,
  )
}

describe('parseCertNotice', () => {
  it.each([
    [
      'app.example.com: expires in 2 days (issuer R3). Renewal is failing: DNS problem.',
      2,
    ],
    [
      'Certificates:\n- app.example.com: expires in 1 day (issuer R3). Automatic renewal is overdue.',
      1,
    ],
    [
      'app.example.com: has expired (issuer R3). The certificate is inside the last-resort window.',
      -1,
    ],
  ])('parses %s', (detail, days) => {
    const n = parseCertNotice(detail)
    expect(n?.domain).toBe('app.example.com')
    expect(n?.daysLeft).toBe(days)
    expect(n?.issuer).toBe('R3')
  })

  it('returns null for unrelated detail', () => {
    expect(parseCertNotice('something else')).toBeNull()
  })
})

describe('CertExpiryInboxCard', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows facts, the connect-channel button and a disabled email toggle when nothing is connected', async () => {
    stubApi({
      channels: [],
      email: { enabled: false, available: false, reason: 'no_email_provider' },
    })
    renderCard('Automatic renewal is overdue')

    expect(screen.getByText('app.example.com')).toBeTruthy()
    expect(screen.getByText('R3')).toBeTruthy()
    expect(screen.getByText('2 days')).toBeTruthy()
    expect(screen.getByText('Automatic renewal is overdue')).toBeTruthy()
    const connect = await screen.findByRole('link', {
      name: 'Connect a notification channel',
    })
    expect(connect.getAttribute('href')).toBe('/settings/notification-channels')
    await waitFor(() => {
      const toggle = screen.getByRole('switch', { name: 'Also email me' })
      expect(
        toggle.getAttribute('aria-disabled') === 'true' ||
          toggle.hasAttribute('disabled') ||
          toggle.hasAttribute('data-disabled'),
      ).toBe(true)
    })
  })

  it('renews through the typed renew endpoint', async () => {
    const renewCalls: string[] = []
    stubApi({
      channels: [{ id: 'c1' }],
      email: { enabled: false, available: true },
      renewCalls,
    })
    renderCard('Automatic renewal is overdue')

    const button = await screen.findByRole('button', { name: 'Renew now' })
    await userEvent.setup().click(button)
    await waitFor(() => {
      expect(renewCalls).toEqual([
        'POST /api/v1/apps/web/domains/app.example.com/cert/renew',
      ])
    })
    expect(
      screen.queryByRole('link', { name: 'Connect a notification channel' }),
    ).toBeNull()
  })

  it('offers Check DNS and retry when the failure is DNS', async () => {
    stubApi({
      channels: [{ id: 'c1' }],
      email: { enabled: false, available: true },
    })
    renderCard('Renewal is failing: DNS problem: NXDOMAIN')

    const link = await screen.findByRole('link', {
      name: 'Check DNS and retry',
    })
    expect(link.getAttribute('href')).toContain('/domains/app.example.com?')
    expect(link.getAttribute('href')).toContain('fix=dns')
  })

  it('prefers the structured failure kind over the failure text', async () => {
    stubApi({
      channels: [{ id: 'c1' }],
      email: { enabled: false, available: true },
    })
    const notice = certNoticeFromItem({
      detail: 'app.example.com: expires in 2 days (issuer R3). Renewal failed.',
      cert: {
        domain: 'app.example.com',
        issuer: 'R3',
        not_after: '2026-11-01T00:00:00Z',
        days_left: 2,
        failure_kind: 'dns',
      },
    })
    expect(notice?.dns).toBe(true)
    renderCard(notice?.reason ?? '', notice?.dns)

    expect(
      await screen.findByRole('link', { name: 'Check DNS and retry' }),
    ).toBeTruthy()
  })
})

describe('certNoticeFromItem', () => {
  const detail =
    'app.example.com: expires in 5 days (issuer R3). Automatic renewal is overdue.'

  it('uses structured fields when present', () => {
    const n = certNoticeFromItem({
      detail,
      cert: {
        domain: 'other.example.com',
        issuer: 'ZeroSSL',
        not_after: '2026-11-01T00:00:00Z',
        days_left: 9,
        failure_kind: 'renewal_failing',
      },
    })
    expect(n).toMatchObject({
      domain: 'other.example.com',
      issuer: 'ZeroSSL',
      daysLeft: 9,
      dns: false,
    })
  })

  it('falls back to the detail text for alerts without structured fields', () => {
    const n = certNoticeFromItem({ detail })
    expect(n).toMatchObject({
      domain: 'app.example.com',
      issuer: 'R3',
      daysLeft: 5,
    })
    expect(n?.dns).toBeUndefined()
    expect(certNoticeFromItem({ detail: 'unrelated' })).toBeNull()
  })
})
