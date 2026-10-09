import { Suspense } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import { CanaryCard } from './CanaryCard'
import deploysEn from '../locales/en/deploys.json'
import type { Brand } from '../types/brand'

vi.mock('@/components/ui/toast', () => ({
  toast: { add: vi.fn() },
}))

vi.mock('../hooks/useBrand', () => ({
  useBrand: (): Brand => ({
    Name: 'Test Brand',
    ShortName: 'testbrand',
    BinaryName: 'testbrand',
    Domain: 'test.example',
    SupportURL: 'https://test.example/support',
    PrimaryColor: '#000000',
    LogoSVG: '',
    DocsURL: 'https://test.example/docs',
    DiscussionsURL: '',
    RepoURL: '',
  }),
}))

// A dedicated instance with the real deploys.json resources loaded
// synchronously, not the app's lazyBackend-driven singleton (src/i18n):
// tests want deterministic, already-loaded translations, proof that the
// real JSON content (not a stub) renders through useTranslation.
const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['deploys'],
  defaultNS: 'deploys',
  resources: { en: { deploys: deploysEn } },
  interpolation: { escapeValue: false },
})

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function renderCard() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <Suspense fallback={<div>loading</div>}>
          <CanaryCard appName="demo-app" />
        </Suspense>
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('CanaryCard', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    vi.mocked(toast.add).mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('starts a canary with the image and weight', async () => {
    fetchMock.mockImplementation(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        Promise.resolve(
          init?.method === 'POST'
            ? json({ active: true, image: 'nginx:2', weight: 25 })
            : json({ active: false, weight: 0 }),
        ),
    )

    const user = userEvent.setup()
    renderCard()
    await user.type(await screen.findByLabelText('Canary image'), 'nginx:2')
    const weight = screen.getByLabelText('Traffic %')
    await user.clear(weight)
    await user.type(weight, '25')
    await user.click(screen.getByRole('button', { name: 'Start canary' }))

    expect(
      await screen.findByText('Canary nginx:2 is receiving 25% of requests.'),
    ).toBeInTheDocument()
    const post = fetchMock.mock.calls.find(
      ([, init]) => (init as RequestInit | undefined)?.method === 'POST',
    )
    expect(JSON.parse((post?.[1] as RequestInit).body as string)).toEqual({
      image: 'nginx:2',
      weight: 25,
    })
  })

  it('promotes a running canary', async () => {
    fetchMock.mockImplementation(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        Promise.resolve(
          init?.method === 'POST'
            ? json({ active: false, weight: 0 })
            : json({ active: true, image: 'nginx:2', weight: 10 }),
        ),
    )

    const user = userEvent.setup()
    renderCard()
    await user.click(await screen.findByRole('button', { name: 'Promote' }))

    await waitFor(() => {
      expect(toast.add).toHaveBeenCalledWith({
        title: 'Canary promoted, deploying.',
        type: 'success',
      })
    })
    expect(
      await screen.findByRole('button', { name: 'Start canary' }),
    ).toBeInTheDocument()
  })
})
