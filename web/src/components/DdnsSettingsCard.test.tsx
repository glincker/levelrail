import { fireEvent, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DdnsSettingsCard } from './DdnsSettingsCard'
import type { DdnsSettings, DdnsStatus } from '../queries/ddns'
import { renderWithQueryI18n } from '@/test/renderWithQuery'

const settings: DdnsSettings = {
  enabled: true,
  provider: 'generic',
  interval_seconds: 300,
  probe_urls: ['https://probe.example/ip'],
  probe_quorum: 2,
  timeout_seconds: 10,
  min_update_gap_seconds: 60,
  reassert_seconds: 86400,
  fail_alert_after: 3,
  allow_private: false,
  allow_insecure_http: false,
  source: 'stored',
  target_configured: true,
  host: 'dyn.example.net',
  running: true,
}

const okStatus: DdnsStatus = {
  running: true,
  last_check: '2026-11-01T10:00:00Z',
  last_ip: '203.0.113.7',
  last_result: 'ok',
  consecutive_failures: 0,
  alert_active: false,
}

function json(body: unknown, status = 200): Response {
  return {
    ok: status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function mockApi(status: DdnsStatus) {
  const fn = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : ''
    if (init?.method === 'PUT') return Promise.resolve(json(settings))
    if (url.endsWith('/status')) return Promise.resolve(json(status))
    return Promise.resolve(json(settings))
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

function putBodies(fn: ReturnType<typeof mockApi>): Record<string, unknown>[] {
  return fn.mock.calls
    .filter(([, init]) => init?.method === 'PUT')
    .map(
      ([, init]) =>
        JSON.parse(typeof init?.body === 'string' ? init.body : '{}') as Record<
          string,
          unknown
        >,
    )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('DdnsSettingsCard', () => {
  it('renders prefilled values, the configured host hint and a running chip', async () => {
    mockApi(okStatus)
    renderWithQueryI18n(<DdnsSettingsCard />)
    expect(
      await screen.findByText('Configured (dyn.example.net)'),
    ).toBeInTheDocument()
    expect(await screen.findByText('Running')).toBeInTheDocument()
    expect(screen.getByText('Saved')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Advanced' }))
    expect(await screen.findByLabelText('Check every (s)')).toHaveValue(300)
  })

  it('shows failing and off chips from status', async () => {
    mockApi({
      ...okStatus,
      last_result: 'failing',
      last_error: 'provider said no',
      consecutive_failures: 2,
    })
    const { unmount } = renderWithQueryI18n(<DdnsSettingsCard />)
    expect(await screen.findByText('Failing')).toBeInTheDocument()
    expect(await screen.findByText('provider said no')).toBeInTheDocument()
    unmount()

    mockApi({ ...okStatus, running: false, last_result: 'not running' })
    renderWithQueryI18n(<DdnsSettingsCard />)
    expect(await screen.findByText('Off')).toBeInTheDocument()
  })

  it('omits target on save unless typed, and sends an empty target only on clear', async () => {
    const fn = mockApi(okStatus)
    renderWithQueryI18n(<DdnsSettingsCard />)
    fireEvent.click(await screen.findByRole('button', { name: 'Advanced' }))
    const interval = await screen.findByLabelText('Check every (s)')

    fireEvent.change(interval, { target: { value: '120' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => {
      expect(putBodies(fn)).toHaveLength(1)
    })
    const first = putBodies(fn)[0]
    expect(first?.interval_seconds).toBe(120)
    expect(first).not.toHaveProperty('target')

    fireEvent.click(screen.getByRole('button', { name: 'Clear' }))
    await waitFor(() => {
      expect(putBodies(fn)).toHaveLength(2)
    })
    expect(putBodies(fn)[1]).toEqual({ target: '' })
  })
})
