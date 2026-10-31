import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { RolloutPolicyCard } from './RolloutPolicyCard'

function json(body: unknown, status = 200): Response {
  return {
    ok: status < 300,
    status,
    json: () => Promise.resolve(body),
  } as unknown as Response
}

afterEach(() => vi.unstubAllGlobals())

describe('RolloutPolicyCard', () => {
  it('prefills the saved policy and PUTs edits', async () => {
    const fetchMock = vi.fn((_url: string, init?: RequestInit) =>
      Promise.resolve(
        init?.method === 'PUT'
          ? json({ max_surge: '2' })
          : json({
              app: 'web',
              strategy: 'rolling',
              replicas: 3,
              policy: { max_surge: '1' },
              progress: null,
            }),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    render(
      <QueryClientProvider client={new QueryClient()}>
        <RolloutPolicyCard appName="web" />
      </QueryClientProvider>,
    )
    const surge = await waitFor(() => {
      const el = document.getElementById('rollout-max_surge')
      if (!el) throw new Error('not rendered')
      return el
    })
    expect(surge).toHaveValue('1')
    await userEvent.clear(surge)
    await userEvent.type(surge, '2')
    await userEvent.click(screen.getByRole('button', { name: /save/i }))
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/v1/apps/web/rollout',
        expect.objectContaining({ method: 'PUT' }),
      ),
    )
  })
  it('does not carry one app draft into another app', async () => {
    const fetchMock = vi.fn((url: string) =>
      Promise.resolve(
        json({
          app: url.includes('/web/') ? 'web' : 'api',
          strategy: 'rolling',
          replicas: 1,
          policy: { max_surge: url.includes('/web/') ? '1' : '5' },
          progress: null,
        }),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    const client = new QueryClient()
    const ui = (name: string) => (
      <QueryClientProvider client={client}>
        <RolloutPolicyCard appName={name} />
      </QueryClientProvider>
    )
    const { rerender } = render(ui('web'))
    const field = () => document.getElementById('rollout-max_surge')
    await waitFor(() => expect(field()).toHaveValue('1'))
    await userEvent.type(field() as HTMLElement, '9')
    rerender(ui('api'))
    await waitFor(() => expect(field()).toHaveValue('5'))
  })
})
