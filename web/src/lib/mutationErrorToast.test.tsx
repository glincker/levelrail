import { cleanup, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import {
  MutationCache,
  QueryClient,
  QueryClientProvider,
  useMutation,
} from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { inlineErrorMeta, type InlineErrorOptions } from './inlineError'
import { toastMutationError } from './mutationErrorToast'

const toastAdd = vi.hoisted(() => vi.fn())
vi.mock('@/components/ui/toast', () => ({ toast: { add: toastAdd } }))

function fail(): Promise<void> {
  return Promise.reject(new Error('boom'))
}

function InlineErrorForm() {
  const save = useMutation({ meta: { errorToast: false }, mutationFn: fail })
  return (
    <div>
      <button type="button" onClick={() => save.mutate()}>
        save
      </button>
      {save.isError ? <p role="alert">{save.error.message}</p> : null}
    </div>
  )
}

function ToastOnlyForm() {
  const save = useMutation({ mutationFn: fail })
  return (
    <button type="button" onClick={() => save.mutate()}>
      save
    </button>
  )
}

// Stands in for a shared hook such as useUpdateApp(name, opts).
function useSharedSave(opts?: InlineErrorOptions) {
  return useMutation({ meta: inlineErrorMeta(opts), mutationFn: fail })
}

function SharedSaveForm({ inline }: { inline: boolean }) {
  const save = useSharedSave(inline ? { inlineError: true } : undefined)
  return (
    <div>
      <button type="button" onClick={() => save.mutate()}>
        save
      </button>
      {save.isError ? <p role="alert">{save.error.message}</p> : null}
    </div>
  )
}

function renderWith(node: React.ReactNode) {
  const client = new QueryClient({
    mutationCache: new MutationCache({
      onError: (error, _v, _c, mutation) => toastMutationError(error, mutation),
    }),
  })
  render(<QueryClientProvider client={client}>{node}</QueryClientProvider>)
}

describe('default mutation error toast', () => {
  afterEach(() => {
    cleanup()
    toastAdd.mockClear()
  })

  it('is skipped when the mutation shows its error inline', async () => {
    renderWith(<InlineErrorForm />)
    await userEvent.click(screen.getByRole('button', { name: 'save' }))
    await screen.findByRole('alert')
    expect(toastAdd).not.toHaveBeenCalled()
  })

  it('still fires for a mutation with no onError and no opt-out', async () => {
    renderWith(<ToastOnlyForm />)
    await userEvent.click(screen.getByRole('button', { name: 'save' }))
    await waitFor(() => expect(toastAdd).toHaveBeenCalledTimes(1))
  })

  it('a shared hook used with inlineError suppresses the toast', async () => {
    renderWith(<SharedSaveForm inline />)
    await userEvent.click(screen.getByRole('button', { name: 'save' }))
    await screen.findByRole('alert')
    expect(toastAdd).not.toHaveBeenCalled()
  })

  it('a shared hook used without the option still toasts', async () => {
    renderWith(<SharedSaveForm inline={false} />)
    await userEvent.click(screen.getByRole('button', { name: 'save' }))
    await waitFor(() => expect(toastAdd).toHaveBeenCalledTimes(1))
  })
})
