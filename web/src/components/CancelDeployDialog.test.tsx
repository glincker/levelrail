import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { toast } from '@/components/ui/toast'
import { CancelDeployDialog } from './CancelDeployDialog'

vi.mock('@/components/ui/toast', () => ({
  toast: { add: vi.fn() },
}))

function respond(status: number, body: unknown) {
  return vi.fn(() =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { 'Content-Type': 'application/json' },
      }),
    ),
  )
}

function renderDialog(running: boolean, onOpenChange = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  render(
    <QueryClientProvider client={client}>
      <CancelDeployDialog
        appName="web"
        deployId="dep_1"
        running={running}
        open
        onOpenChange={onOpenChange}
      />
    </QueryClientProvider>,
  )
  return onOpenChange
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.clearAllMocks()
})

describe('CancelDeployDialog', () => {
  it('says a running build is stopped and the live release is untouched', () => {
    renderDialog(true)
    expect(screen.getByText(/Stops the build in progress/)).toBeInTheDocument()
  })

  it('says a queued deploy is removed from the queue', () => {
    renderDialog(false)
    expect(
      screen.getByText('Removes this deploy from the queue.'),
    ).toBeInTheDocument()
  })

  it('keeps the deploy and closes without calling the API', async () => {
    const fetchMock = respond(200, {})
    vi.stubGlobal('fetch', fetchMock)
    const onOpenChange = renderDialog(true)
    await userEvent.click(
      screen.getByRole('button', { name: 'Keep it running' }),
    )
    expect(onOpenChange).toHaveBeenCalledWith(false)
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('posts the cancel, toasts and closes on success', async () => {
    const fetchMock = respond(200, { id: 'dep_1', status: 'canceled' })
    vi.stubGlobal('fetch', fetchMock)
    const onOpenChange = renderDialog(true)
    await userEvent.click(screen.getByRole('button', { name: 'Cancel deploy' }))
    await waitFor(() => {
      expect(onOpenChange).toHaveBeenCalledWith(false)
    })
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/apps/web/deploys/dep_1/cancel',
      expect.objectContaining({ method: 'POST' }),
    )
    expect(toast.add).toHaveBeenCalledWith(
      expect.objectContaining({ title: 'Deploy canceled.', type: 'success' }),
    )
  })

  it('shows the server error and stays open on failure', async () => {
    vi.stubGlobal('fetch', respond(409, { error: 'deploy already finished' }))
    const onOpenChange = renderDialog(true)
    await userEvent.click(screen.getByRole('button', { name: 'Cancel deploy' }))
    expect(
      await screen.findByText('deploy already finished'),
    ).toBeInTheDocument()
    expect(onOpenChange).not.toHaveBeenCalledWith(false)
  })
})
