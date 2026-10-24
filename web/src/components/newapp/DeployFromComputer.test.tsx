import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Brand } from '../../types/brand'
import { NewAppI18n } from '../../test/newAppI18n'
import { jsonResponse } from '../../test/newAppFixtures'
import { DeployFromComputer } from './DeployFromComputer'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))

vi.mock('../../hooks/useBrand', () => ({
  useBrand: (): Brand => ({
    Name: 'Test Brand',
    ShortName: 'testbrand',
    BinaryName: 'testbrand',
    Domain: 'test.example',
    SupportURL: '',
    PrimaryColor: '#000000',
    LogoSVG: '',
    DocsURL: '',
    DiscussionsURL: '',
    RepoURL: '',
  }),
}))

afterEach(() => {
  vi.restoreAllMocks()
})

function renderPanel() {
  render(
    <NewAppI18n>
      <QueryClientProvider client={new QueryClient()}>
        <DeployFromComputer />
      </QueryClientProvider>
    </NewAppI18n>,
  )
}

describe('DeployFromComputer', () => {
  it('shows the deploy command with this control plane URL filled in', () => {
    renderPanel()
    expect(
      screen.getByRole('heading', { name: 'Deploy from your computer' }),
    ).toBeInTheDocument()
    expect(
      screen.getByText(
        `testbrand-cli deploy --api-url ${window.location.origin}`,
      ),
    ).toBeInTheDocument()
  })

  it('creates a token on click, shows it once, then hides it', async () => {
    const user = userEvent.setup()
    const spy = vi
      .spyOn(globalThis, 'fetch')
      .mockImplementation(() =>
        Promise.resolve(jsonResponse({ id: 't1', token: 'lr_secret' }, 201)),
      )
    renderPanel()
    await user.click(
      screen.getByRole('button', { name: 'Create a deploy token' }),
    )
    expect(
      await screen.findByText('export APP_API_TOKEN=lr_secret'),
    ).toBeInTheDocument()
    const init = spy.mock.calls[0]?.[1]
    const raw = typeof init?.body === 'string' ? init.body : '{}'
    const body = JSON.parse(raw) as {
      abilities: string[]
    }
    expect(body.abilities).toContain('deploy')
    await user.click(
      screen.getByRole('button', { name: 'I copied them, hide' }),
    )
    await waitFor(() => {
      expect(screen.queryByText(/lr_secret/)).toBeNull()
    })
  })

  it('offers Settings when the token cannot be created', async () => {
    const user = userEvent.setup()
    vi.spyOn(globalThis, 'fetch').mockImplementation(() =>
      Promise.resolve(jsonResponse({ error: 'forbidden' }, 403)),
    )
    renderPanel()
    await user.click(
      screen.getByRole('button', { name: 'Create a deploy token' }),
    )
    expect(
      await screen.findByRole('link', { name: 'Create one in Settings' }),
    ).toHaveAttribute('href', '/settings/tokens')
  })
})
