import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CreateEnvironmentDialog } from './CreateEnvironmentDialog'

afterEach(() => {
  vi.unstubAllGlobals()
})

function renderDialog() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <CreateEnvironmentDialog projectId="proj1" />
    </QueryClientProvider>,
  )
}

// A second simple single-step migration target, picked specifically
// because it adds a checkbox alongside the one text field: proof
// CreateFlowShell's children slot stays free-form rather than assuming
// every create flow is a single input.
describe('CreateEnvironmentDialog', () => {
  it('submits the name and protected flag together', async () => {
    const bodies: unknown[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init?: RequestInit) => {
        if (init?.body) bodies.push(JSON.parse(init.body as string))
        return Promise.resolve({
          ok: true,
          status: 201,
          json: () => Promise.resolve({ id: 'e1', name: 'staging' }),
        } as unknown as Response)
      }),
    )
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('button', { name: /new environment/i }))
    const dialog = screen.getByRole('dialog')
    await user.type(within(dialog).getByLabelText('Name'), 'staging')
    await user.click(screen.getByText(/Protected: require confirmation/i))
    await user.click(
      within(dialog).getByRole('button', { name: 'Create environment' }),
    )

    await waitFor(() => {
      expect(bodies).toHaveLength(1)
    })
    expect(bodies[0]).toEqual({ name: 'staging', protected: true })
  })
})
