import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
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

function renderScreen(setup?: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={client}>
      <LoginScreen setup={setup} />
    </QueryClientProvider>,
  )
}

describe('LoginScreen', () => {
  beforeEach(() => {
    vi.mocked(useBrand).mockReturnValue(brand)
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

  it('opens the setup form with the token pre-filled from ?setup=', async () => {
    mockFetch(false)
    renderScreen('tok-from-installer')
    expect(await findTitle('Set up the admin account')).toBeInTheDocument()
    expect(screen.getByLabelText('Setup token')).toHaveValue(
      'tok-from-installer',
    )
    expect(screen.getByText(/sudo testbin setup-token/)).toBeInTheDocument()
  })
})
