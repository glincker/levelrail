import { renderHook, waitFor } from '@testing-library/react'
import {
  MutationCache,
  QueryClient,
  QueryClientProvider,
} from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { toastMutationError } from '../lib/mutationErrorToast'
import {
  useBeginPasskeyLogin,
  useDiscoverablePasskeyLogin,
  useFinishPasskeyLogin,
} from './passkeys'

const toastAdd = vi.hoisted(() => vi.fn())

vi.mock('@/components/ui/toast', () => ({ toast: { add: toastAdd } }))
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
  useRouter: () => ({ navigate: vi.fn() }),
}))

function wrapper() {
  const qc = new QueryClient({
    mutationCache: new MutationCache({
      onError: (error, _v, _c, mutation) => {
        toastMutationError(error, mutation)
      },
    }),
  })
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  )
}

describe('passkey sign-in mutations', () => {
  it('never raise the global error toast (the sign-in screen renders errors inline)', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(new Response('{}', { status: 500 })),
    )
    const w = wrapper()
    const begin = renderHook(() => useBeginPasskeyLogin(), { wrapper: w })
    const finish = renderHook(() => useFinishPasskeyLogin(), { wrapper: w })
    const disc = renderHook(() => useDiscoverablePasskeyLogin(), {
      wrapper: w,
    })
    begin.result.current.mutate('dev')
    finish.result.current.mutate({} as never)
    disc.result.current.mutate({ conditional: true })
    await waitFor(() => {
      expect(begin.result.current.isError).toBe(true)
      expect(finish.result.current.isError).toBe(true)
      expect(disc.result.current.isError).toBe(true)
    })
    expect(toastAdd).not.toHaveBeenCalled()
    vi.unstubAllGlobals()
  })
})
