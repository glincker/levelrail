import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import userEvent from '@testing-library/user-event'
import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Brand } from '../types/brand'
import { CommonI18n } from '../test/commonI18n'

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => vi.fn(),
    Link: ({ children }: { children: React.ReactNode }) => <a>{children}</a>,
  }
})

vi.mock('../hooks/useBrand', () => ({ useBrand: vi.fn() }))

import { useBrand } from '../hooks/useBrand'
import { LoginScreen } from './LoginScreen'
import { ThemeProvider } from './ThemeProvider'

const brand: Brand = {
  Name: 'Test Brand',
  ShortName: 'testbrand',
  BinaryName: 'testbin',
  Domain: 'test.example',
  SupportURL: '',
  PrimaryColor: '',
  LogoSVG: '',
  DocsURL: '',
  DiscussionsURL: '',
  RepoURL: '',
}

function mockFetch(needsSetup: boolean) {
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL) => {
      const url = input instanceof Request ? input.url : String(input)
      const body = url.endsWith('/api/v1/auth/setup-status')
        ? { needs_setup: needsSetup }
        : url.includes('/oauth/providers')
          ? []
          : { enabled: false }
      return Promise.resolve(
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }),
  )
}

function findTitle(text: string) {
  return screen.findByRole('heading', { level: 1, name: text })
}

function renderScreen(setup?: string, sessionEnded = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <CommonI18n>
        <ThemeProvider>
          <LoginScreen setup={setup} sessionEnded={sessionEnded} />
        </ThemeProvider>
      </CommonI18n>
    </QueryClientProvider>,
  )
}

describe('LoginScreen', () => {
  beforeEach(() => {
    vi.mocked(useBrand).mockReturnValue(brand)
    window.localStorage.clear()
    // jsdom has no matchMedia; ThemeProvider (via ThemeToggle) needs it.
    window.matchMedia = vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    }))
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('switches to the setup form when the instance has no admin yet', async () => {
    mockFetch(true)
    renderScreen()
    expect(await findTitle('Set up the admin account')).toBeInTheDocument()
    expect(screen.getByLabelText('Setup token')).toHaveValue('')
  })

  it('stays on sign in once an admin exists', async () => {
    mockFetch(false)
    renderScreen()
    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith('/api/v1/auth/setup-status')
    })
    expect(await findTitle('Sign in')).toBeInTheDocument()
    expect(screen.queryByLabelText('Setup token')).not.toBeInTheDocument()
  })

  it('hides the setup admin account tab once an admin already exists', async () => {
    mockFetch(false)
    renderScreen()
    await findTitle('Sign in')
    await waitFor(() => {
      expect(
        screen.queryByRole('tab', { name: 'Set up admin account' }),
      ).not.toBeInTheDocument()
    })
    expect(screen.getAllByLabelText('Username')).toHaveLength(1)
  })

  it('opens the setup form with the token pre-filled from the installer link', async () => {
    mockFetch(false)
    renderScreen('tok-from-installer')
    expect(await findTitle('Set up the admin account')).toBeInTheDocument()
    expect(screen.getByLabelText('Setup token')).toHaveValue(
      'tok-from-installer',
    )
    expect(screen.getByText(/sudo testbin setup-token/)).toBeInTheDocument()
  })

  it('tells a locked-out user how to recover from the server', async () => {
    vi.mocked(useBrand).mockReturnValue({
      ...brand,
      DocsURL: 'https://docs.test.example',
    })
    mockFetch(false)
    renderScreen()
    expect(await findTitle('Sign in')).toBeInTheDocument()
    expect(screen.queryByText(/recover-admin/)).toBeNull()
    await userEvent.click(
      screen.getByRole('button', { name: /Locked out\?|lockedOut\.link/ }),
    )
    expect(
      await screen.findByText(
        'sudo testbin recover-admin --username <your username>',
      ),
    ).toBeInTheDocument()
    const link = screen.getByRole('link', {
      name: /Emergency access|emergencyDocs/,
    })
    expect(link).toHaveAttribute(
      'href',
      'https://docs.test.example/emergency-access',
    )
  })

  it('shows a quiet attribution footer with safe external links', async () => {
    mockFetch(false)
    renderScreen()
    expect(await findTitle('Sign in')).toBeInTheDocument()
    const link = screen.getByRole('link', { name: /TheAuth|theAuthLabel/ })
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('does not show the recovery hint while setting up the first admin', async () => {
    mockFetch(true)
    renderScreen()
    expect(await findTitle('Set up the admin account')).toBeInTheDocument()
    expect(
      screen.queryByText('sudo testbin recover-admin'),
    ).not.toBeInTheDocument()
  })

  it('explains a sign-out caused by an ended session', async () => {
    mockFetch(false)
    renderScreen(undefined, true)
    expect(await findTitle('Sign in')).toBeInTheDocument()
    expect(screen.getByRole('status')).toHaveTextContent(
      /signed out|sessionEnded\.notice/,
    )
  })

  it('shows no sign-out notice on a normal visit', async () => {
    mockFetch(false)
    renderScreen()
    expect(await findTitle('Sign in')).toBeInTheDocument()
    expect(
      screen.queryByText(/signed out|sessionEnded\.notice/),
    ).not.toBeInTheDocument()
  })

  it('shows the brand panel beside the form', async () => {
    renderScreen()
    await findTitle('Sign in')
    expect(
      screen.getByRole('complementary', { name: /^About / }),
    ).toBeInTheDocument()
  })
})
