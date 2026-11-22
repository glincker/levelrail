import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { useApiMutation } from './useApiMutation'

function setup() {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  const spy = vi.spyOn(client, 'invalidateQueries')
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
  return { spy, wrapper }
}

describe('useApiMutation', () => {
  it('invalidates static keys on success and forwards onSuccess', async () => {
    const { spy, wrapper } = setup()
    const onSuccess = vi.fn()
    const { result } = renderHook(
      () =>
        useApiMutation({
          fn: (n: number) => Promise.resolve(n * 2),
          invalidate: [['a'], ['b', 1]],
          onSuccess,
        }),
      { wrapper },
    )
    await act(async () => {
      await result.current.mutateAsync(2)
    })
    expect(spy).toHaveBeenCalledWith({ queryKey: ['a'] })
    expect(spy).toHaveBeenCalledWith({ queryKey: ['b', 1] })
    expect(onSuccess).toHaveBeenCalledWith(4, 2)
  })

  it('derives keys from vars and data', async () => {
    const { spy, wrapper } = setup()
    const { result } = renderHook(
      () =>
        useApiMutation({
          fn: (id: string) => Promise.resolve({ ok: id }),
          invalidate: (vars, data) => [['item', vars, data.ok]],
        }),
      { wrapper },
    )
    await act(async () => {
      await result.current.mutateAsync('x')
    })
    expect(spy).toHaveBeenCalledWith({ queryKey: ['item', 'x', 'x'] })
  })

  it('skips invalidation on error and forwards onError', async () => {
    const { spy, wrapper } = setup()
    const onError = vi.fn()
    const { result } = renderHook(
      () =>
        useApiMutation({
          fn: () => Promise.reject(new Error('boom')),
          invalidate: [['a']],
          onError,
        }),
      { wrapper },
    )
    await act(async () => {
      await result.current.mutateAsync().catch(() => undefined)
    })
    await waitFor(() => expect(onError).toHaveBeenCalled())
    expect(spy).not.toHaveBeenCalled()
  })
})
