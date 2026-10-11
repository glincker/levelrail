import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import commonEn from '../locales/en/common.json'
import signInEn from '../locales/en/signIn.json'
import type { Brand } from '../types/brand'

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

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'signIn'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, signIn: signInEn } },
  interpolation: { escapeValue: false },
})

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
  return screen.findByText(text, { selector: '[data-slot="card-title"]' })
}

function renderScreen(setup?: string, sessionEnded = false) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <ThemeProvider>
          <LoginScreen setup={setup} sessionEnded={sessionEnded} />
        </ThemeProvider>
      </I18nextProvider>
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

  it('opens the setup form with the token pre-filled from ?setup=', async () => {
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
    expect(screen.getByText('sudo testbin recover-admin')).toBeInTheDocument()
    const link = screen.getByRole('link', {
      name: /Emergency access|emergencyDocs/,
    })
    expect(link).toHaveAttribute(
      'href',
      'https://docs.test.example/emergency-access',
    )
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

  it('has one level-1 heading, a brand mark and no legacy amber styling', async () => {
    mockFetch(false)
    const { container } = renderScreen()
    expect(await findTitle('Sign in')).toBeInTheDocument()
    expect(
      screen.getAllByRole('heading', { level: 1, name: 'Sign in' }),
    ).toHaveLength(1)
    expect(screen.getByText('testbrand')).toBeInTheDocument()
    expect(container.innerHTML).not.toMatch(/amber|245 158 11|245_158_11/)
    expect(container.querySelector('[style]')).toBeNull()
  })

  it('labels the sign-in fields for password managers', async () => {
    mockFetch(false)
    renderScreen()
    expect(await findTitle('Sign in')).toBeInTheDocument()
    expect(screen.getByLabelText('Username')).toHaveAttribute(
      'autocomplete',
      'username webauthn',
    )
  })
})
