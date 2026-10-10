import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CreateTokenDialog } from './CreateTokenDialog'
import settingsEn from '../locales/en/settings.json'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function renderDialog() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={testI18n}>
        <CreateTokenDialog />
      </I18nextProvider>
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

  it('mints a sign-in approver token from the dedicated checkbox alone', async () => {
    const bodies: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn((_input: string, init?: RequestInit) => {
        bodies.push(typeof init?.body === 'string' ? init.body : '{}')
        return Promise.resolve({
          ok: true,
          status: 201,
          json: () =>
            Promise.resolve({ id: 't2', name: 'approver', token: 'tok' }),
        } as unknown as Response)
      }),
    )
    const user = userEvent.setup()
    renderDialog()
    await user.click(screen.getByRole('button', { name: 'Create token' }))
    await user.type(screen.getByLabelText('Name'), 'approver')
    await user.click(
      screen.getByRole('checkbox', { name: /Approve my sign-ins/ }),
    )
    await user.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'Create token',
      }),
    )
    await waitFor(() => {
      expect(bodies.length).toBe(1)
    })
    const body = JSON.parse(bodies[0] ?? '{}') as {
      abilities: string[]
      expires_in_days?: number
    }
    expect(body.abilities).toEqual(['signin:approve'])
    expect(body.expires_in_days).toBe(30)
  })

  it('refuses a sign-in approver token for an agent', async () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    const user = userEvent.setup()
    renderDialog()
    await user.click(screen.getByRole('button', { name: 'Create token' }))
    await user.type(screen.getByLabelText('Name'), 'approver')
    await user.type(screen.getByLabelText('Agent name (optional)'), 'bot')
    await user.click(
      screen.getByRole('checkbox', { name: /Approve my sign-ins/ }),
    )
    await user.click(
      within(screen.getByRole('dialog')).getByRole('button', {
        name: 'Create token',
      }),
    )
    expect(
      await screen.findByText(
        'A token that approves sign-ins cannot be issued to an agent',
      ),
    ).toBeInTheDocument()
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
