import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CreateProjectDialog } from './CreateProjectDialog'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return { ...actual, useNavigate: () => vi.fn() }
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function renderDialog() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <CreateProjectDialog />
    </QueryClientProvider>,
  )
}

// End-to-end proof that CreateFlowShell's chrome (header, form submit,
// error banner, pending state) behaves correctly for the simplest
// single-step migration target: open, fill, submit, see success.
describe('CreateProjectDialog', () => {
  it('opens, submits the name, and closes on success', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 201,
        json: () => Promise.resolve({ id: 'p1', name: 'my-saas' }),
      } as unknown as Response),
    )
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('button', { name: /new project/i }))
    const dialog = screen.getByRole('dialog')
    expect(within(dialog).getByText('New project')).toBeInTheDocument()

    const submitButton = within(dialog).getByRole('button', {
      name: 'Create project',
    })
    expect(submitButton).toBeDisabled()

    await user.type(screen.getByLabelText('Name'), 'my-saas')
    expect(submitButton).toBeEnabled()
    await user.click(submitButton)

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/v1/projects',
      expect.objectContaining({ method: 'POST' }),
    )
  })

  it('shows the standardized error banner on failure', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve({
          ok: false,
          status: 500,
          json: () => Promise.resolve({ error: 'database is unavailable' }),
        } as unknown as Response),
      ),
    )
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('button', { name: /new project/i }))
    await user.type(screen.getByLabelText('Name'), 'my-saas')
    await user.click(screen.getByRole('button', { name: 'Create project' }))

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('database is unavailable')
  })
})
