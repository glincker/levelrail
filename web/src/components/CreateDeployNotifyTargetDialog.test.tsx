import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CreateDeployNotifyTargetDialog } from './CreateDeployNotifyTargetDialog'

afterEach(() => {
  vi.unstubAllGlobals()
})

// Mirrors CreateAlertRuleDialog.test.tsx's own pickOption: a base-ui
// Select's option only commits a plain click as a real selection once a
// pointerdown on that same item preceded it, which a real click always
// synthesizes but fireEvent.click alone does not. Retries the open+click
// pair since the popup can take a render pass to attach.
async function pickOption(trigger: HTMLElement, optionText: string) {
  const maxAttempts = 5
  for (let attempt = 1; attempt <= maxAttempts; attempt++) {
    fireEvent.click(trigger)
    try {
      const option = screen.getByText(optionText)
      fireEvent.pointerDown(option, { pointerType: 'mouse' })
      fireEvent.click(option)
      return
    } catch (err) {
      if (attempt === maxAttempts) throw err
      await new Promise((resolve) => setTimeout(resolve, 20))
    }
  }
}

function renderDialog() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <CreateDeployNotifyTargetDialog appName="web" />
    </QueryClientProvider>,
  )
}

function stubFetch(channels: unknown[], bodies: unknown[]) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      if (String(url).includes('/notification-channels')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve(channels),
        } as unknown as Response)
      }
      if (init?.body) bodies.push(JSON.parse(init.body as string))
      return Promise.resolve({
        ok: true,
        status: 201,
        json: () => Promise.resolve({ id: 'target1' }),
      } as unknown as Response)
    }),
  )
}

// Exercises both of this dialog's branches: the empty-channels state
// (a Close-only footer, no form at all) and the real submit path, which
// also fixes a pre-migration bug where this dialog had no actual <form>
// element despite looking like one.
describe('CreateDeployNotifyTargetDialog', () => {
  it('shows a close-only empty state when no channels are connected', async () => {
    stubFetch([], [])
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('button', { name: 'Add target' }))
    await screen.findByText(/No channels connected yet/)
    expect(
      screen.queryByRole('button', { name: /^Add target$/ }),
    ).not.toBeInTheDocument()

    // Two "Close" matches: the explicit footer button and the dialog
    // frame's own sr-only-labeled X button; the footer one renders first.
    const [closeButton] = screen.getAllByRole('button', { name: 'Close' })
    await user.click(closeButton!)
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    })
  })

  it('submits the chosen channel and enabled flag', async () => {
    const bodies: unknown[] = []
    stubFetch([{ id: 'c1', name: 'Team Slack', kind: 'slack' }], bodies)
    const user = userEvent.setup()
    renderDialog()

    await user.click(screen.getByRole('button', { name: 'Add target' }))
    const dialog = await screen.findByRole('dialog')
    const combobox = within(dialog).getByRole('combobox')
    await pickOption(combobox, 'Team Slack (Slack)')

    await user.click(within(dialog).getByRole('button', { name: 'Add target' }))

    await waitFor(() => {
      expect(bodies).toHaveLength(1)
    })
    expect(bodies[0]).toEqual({ channel_id: 'c1', enabled: true })
  })
})
