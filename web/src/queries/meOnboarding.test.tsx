import type { ReactNode } from 'react'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  OnboardingTransitionError,
  fetchMeOnboarding,
  onboardingActive,
  useMeOnboarding,
  useSetOnboarding,
  type MeOnboarding,
} from './meOnboarding'

const base: MeOnboarding = {
  state: 'not_started',
  current_step: 'welcome',
  flow_version: 1,
  enabled: true,
}

function reply(status: number, body: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve(new Response(JSON.stringify(body), { status })),
    ),
  )
}

function wrapper({ children }: { children: ReactNode }) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return <QueryClientProvider client={qc}>{children}</QueryClientProvider>
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('meOnboarding', () => {
  it('stays invisible when the route is missing', async () => {
    reply(404, { error: 'not found' })
    expect(await fetchMeOnboarding()).toBeNull()
    const { result } = renderHook(() => useMeOnboarding(), { wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(onboardingActive(result.current.data)).toBe(false)
  })

  it('stays invisible when the flow is disabled', async () => {
    reply(200, { ...base, enabled: false })
    const { result } = renderHook(() => useMeOnboarding(), { wrapper })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(onboardingActive(result.current.data)).toBe(false)
  })

  it('is active when enabled', async () => {
    reply(200, base)
    expect(onboardingActive(await fetchMeOnboarding())).toBe(true)
  })

  it('surfaces invalid_transition as a typed error', async () => {
    reply(409, { error: 'cannot move', code: 'invalid_transition' })
    const { result } = renderHook(() => useSetOnboarding(), { wrapper })
    result.current.mutate({ state: 'completed' })
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(result.current.error).toBeInstanceOf(OnboardingTransitionError)
  })
})
