import type { ReactNode } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useMetricSeries } from './metrics'
import { createTestQueryClient } from '@/test/testQueryClient'

const series = JSON.stringify({ points: [{ value: 1 }] })

function wrapperFor() {
  const client = createTestQueryClient()
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
}

describe('useMetricSeries', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('keeps the last reading while a sliding window refetches', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(new Response(series, { status: 200 }))),
    )
    const range = (minute: number) => ({
      from: new Date(Date.UTC(2026, 0, 1, 0, minute)),
      to: new Date(Date.UTC(2026, 0, 1, 0, minute + 15)),
    })
    const { result, rerender } = renderHook(
      ({ minute }) => useMetricSeries('web', 'cpu_percent', range(minute)),
      { wrapper: wrapperFor(), initialProps: { minute: 0 } },
    )
    await waitFor(() => expect(result.current.data).toBeDefined())

    rerender({ minute: 1 })
    expect(result.current.isPending).toBe(false)
    expect(result.current.data).toBeDefined()
  })

  it('does not show another app reading while switching apps', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(new Response(series, { status: 200 }))),
    )
    const range = {
      from: new Date(Date.UTC(2026, 0, 1, 0, 0)),
      to: new Date(Date.UTC(2026, 0, 1, 0, 15)),
    }
    const { result, rerender } = renderHook(
      ({ app }) => useMetricSeries(app, 'cpu_percent', range),
      { wrapper: wrapperFor(), initialProps: { app: 'web' } },
    )
    await waitFor(() => expect(result.current.data).toBeDefined())

    rerender({ app: 'api' })
    expect(result.current.data).toBeUndefined()
  })
})
