import { fireEvent, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { IngressDns01Switch } from './IngressDns01Switch'
import type { IngressDns01 } from '../queries/ingressDns01'
import { renderWithQueryI18n } from '@/test/renderWithQuery'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ to, children }: { to: string; children: ReactNode }) => (
    <a href={to}>{children}</a>
  ),
}))

const toastError = vi.hoisted(() => vi.fn())
vi.mock('@/components/ui/toast', () => ({
  toast: { add: toastError },
}))

function json(body: unknown, status = 200): Response {
  return {
    ok: status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

function mockApi(state: IngressDns01, putStatus = 200) {
  const fn = vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
    if (init?.method === 'PUT') {
      if (putStatus >= 400) {
        return Promise.resolve(
          json({ error: 'connect a DNS provider first' }, putStatus),
        )
      }
      const enabled = JSON.parse(
        typeof init.body === 'string' ? init.body : '{}',
      ) as { enabled: boolean }
      return Promise.resolve(json({ ...state, enabled: enabled.enabled }))
    }
    return Promise.resolve(json(state))
  })
  vi.stubGlobal('fetch', fn)
  return fn
}

afterEach(() => {
  vi.unstubAllGlobals()
  toastError.mockClear()
})

const base: IngressDns01 = {
  enabled: false,
  source: 'default',
  provider_connected: true,
}

describe('IngressDns01Switch', () => {
  it('renders the label and helper, off by default', async () => {
    mockApi(base)
    renderWithQueryI18n(<IngressDns01Switch />)
    const sw = await screen.findByRole('switch', {
      name: 'Use DNS challenge for every host',
    })
    expect(sw).toHaveAttribute('aria-checked', 'false')
    expect(
      screen.getByText(/Needs a connected DNS provider/),
    ).toBeInTheDocument()
  })

  it('toggling sends PUT with enabled true', async () => {
    const fn = mockApi(base)
    renderWithQueryI18n(<IngressDns01Switch />)
    fireEvent.click(await screen.findByRole('switch'))
    await waitFor(() => {
      const put = fn.mock.calls.find(([, i]) => i?.method === 'PUT')
      expect(put?.[1]?.body).toBe(JSON.stringify({ enabled: true }))
    })
    await waitFor(() =>
      expect(screen.getByRole('switch')).toHaveAttribute(
        'aria-checked',
        'true',
      ),
    )
  })

  it('is disabled with a link when no provider is connected', async () => {
    mockApi({ ...base, provider_connected: false })
    renderWithQueryI18n(<IngressDns01Switch />)
    expect(await screen.findByRole('switch')).toHaveAttribute('data-disabled')
    expect(screen.getByRole('link', { name: 'Connect one' })).toHaveAttribute(
      'href',
      '/settings/platform/dns-providers',
    )
  })

  it('rolls back and toasts when the server refuses', async () => {
    mockApi(base, 400)
    renderWithQueryI18n(<IngressDns01Switch />)
    fireEvent.click(await screen.findByRole('switch'))
    await waitFor(() => expect(toastError).toHaveBeenCalled())
    expect(screen.getByRole('switch')).toHaveAttribute('aria-checked', 'false')
  })
})
