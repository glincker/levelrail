import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { useInvalidatingMutation } from './useInvalidatingMutation'

function setup() {
  const client = new QueryClient()
  const spy = vi.spyOn(client, 'invalidateQueries')
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
  return { spy, wrapper }
}

describe('useInvalidatingMutation', () => {
  it('invalidates every returned key after success', async () => {
    const { spy, wrapper } = setup()
    const { result } = renderHook(
      () =>
        useInvalidatingMutation({
          mutationFn: (id: string) => Promise.resolve({ id }),
          invalidate: (data, id) => [['things'], ['thing', data.id, id]],
        }),
      { wrapper },
    )
    await act(async () => {
      await result.current.mutateAsync('a')
    })
    expect(spy).toHaveBeenCalledWith({ queryKey: ['things'] })
    expect(spy).toHaveBeenCalledWith({ queryKey: ['thing', 'a', 'a'] })
  })

  it('does not invalidate when the mutation fails', async () => {
    const { spy, wrapper } = setup()
    const { result } = renderHook(
      () =>
        useInvalidatingMutation({
          mutationFn: () => Promise.reject(new Error('boom')),
          invalidate: () => [['things']],
        }),
      { wrapper },
    )
    await act(async () => {
      await result.current.mutateAsync().catch(() => undefined)
    })
    await waitFor(() => expect(result.current.isError).toBe(true))
    expect(spy).not.toHaveBeenCalled()
  })
})
