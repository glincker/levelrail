import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { AppDetail } from '../../types/appDetail'
import { resetPowerTransitions } from '../apps/appPower'
import { useSuggestionActions } from './useSuggestionActions'

function urlOf(input: RequestInfo | URL): string {
  if (typeof input === 'string') return input
  return input instanceof URL ? input.href : input.url
}

vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))

const app = { name: 'web', image: 'web:v1', env_dirty: true } as AppDetail

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

function mockFetch() {
  return vi.spyOn(globalThis, 'fetch').mockImplementation((input) =>
    Promise.resolve({
      ok: true,
      status: 202,
      json: () =>
        Promise.resolve(urlOf(input).endsWith('/deploys') ? [] : { ...app }),
    } as unknown as Response),
  )
}

describe('useSuggestionActions', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    window.localStorage.clear()
    resetPowerTransitions()
  })

  it('apply_pending posts to the apply-pending endpoint', async () => {
    const spy = mockFetch()
    const { result } = renderHook(() => useSuggestionActions(app), { wrapper })
    act(() => result.current.run.apply_pending())
    await waitFor(() => expect(spy).toHaveBeenCalled())
    const [url, init] = spy.mock.calls[0] ?? []
    expect(url).toBe('/api/v1/apps/web/apply-pending')
    expect((init as RequestInit).method).toBe('POST')
  })

  it('restart posts to the restart endpoint (env_dirty fallback)', async () => {
    window.localStorage.setItem('app-power:skip-restart-confirm', '1')
    const spy = mockFetch()
    const { result } = renderHook(() => useSuggestionActions(app), { wrapper })
    act(() => result.current.run.restart())
    await waitFor(() => expect(spy).toHaveBeenCalled())
    expect(spy.mock.calls[0]?.[0]).toBe('/api/v1/apps/web/restart')
  })

  it('open_health navigates to health settings without mutating', () => {
    const spy = mockFetch()
    const { result } = renderHook(() => useSuggestionActions(app), { wrapper })
    act(() => result.current.run.open_health())
    expect(spy).not.toHaveBeenCalled()
  })
})
