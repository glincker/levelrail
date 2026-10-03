import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CreateTokenDialog } from './CreateTokenDialog'

afterEach(() => {
  vi.unstubAllGlobals()
})

function renderDialog() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <CreateTokenDialog />
    </QueryClientProvider>,
  )
}

// CreateTokenDialog is the proof-of-concept for CreateFlowKit's step
// indicator: step 1 is the details form, step 2 is the one-time secret
// reveal (TokenCreatedView), rendered by two different components that
// both read from the same CreateFlowSteps list.
describe('CreateTokenDialog', () => {
  it('walks step 1 (details) into step 2 (reveal) on success', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        Promise.resolve({
          ok: true,
          status: 201,
          json: () =>
            Promise.resolve({
              id: 't1',
              name: 'ci-deploy',
              token: 'levelrail_sk_abc123',
            }),
        } as unknown as Response),
      ),
    )
    const user = userEvent.setup()
    // user-event's own setup() installs a clipboard stub for keyboard
    // copy/paste simulation, overwriting whatever is defined before it;
    // ours has to come after to actually take effect.
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText },
      configurable: true,
    })
    renderDialog()

    await user.click(screen.getByRole('button', { name: 'Create token' }))
    expect(screen.getByLabelText('Step 1 of 2: Details')).toBeInTheDocument()

    await user.type(screen.getByLabelText('Name'), 'ci-deploy')
    // At least one ability is required before the form can submit.
    const abilityCheckboxes = screen.getAllByRole('checkbox')
    await user.click(abilityCheckboxes[0]!)

    await user.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'Create token',
      }),
    )

    await waitFor(() => {
      expect(
        screen.getByLabelText('Step 2 of 2: Save your token'),
      ).toBeInTheDocument()
    })
    expect(screen.getByText('Token created')).toBeInTheDocument()
    expect(screen.getByText('levelrail_sk_abc123')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Copy' }))
    expect(writeText).toHaveBeenCalledWith('levelrail_sk_abc123')

    await user.click(screen.getByRole('button', { name: 'Done' }))
    await waitFor(() => {
      expect(screen.queryByText('Token created')).not.toBeInTheDocument()
    })
  })
})
