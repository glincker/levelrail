import { Suspense } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import { ImageAutoUpdateCard } from './ImageAutoUpdateCard'
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
          <ImageAutoUpdateCard appName="demo-app" />
        </Suspense>
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('ImageAutoUpdateCard', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    vi.mocked(toast.add).mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('renders translated copy and the never-checked state', async () => {
    fetchMock.mockResolvedValue(json({ enabled: false }))

    renderCard()

    expect(
      await screen.findByText('Update when the image changes'),
    ).toBeInTheDocument()
    expect(screen.getByText('Not checked yet.')).toBeInTheDocument()
    expect(
      screen.getByRole('switch', { name: 'Image auto-update enabled' }),
    ).not.toBeChecked()
  })

  it('shows the result of a manual check', async () => {
    fetchMock.mockImplementation(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        Promise.resolve(
          init?.method === 'POST'
            ? json({
                enabled: false,
                last_result: 'redeployed with a newer image',
                last_checked_at: '2026-10-08T12:00:00Z',
              })
            : json({ enabled: false }),
        ),
    )

    const user = userEvent.setup()
    renderCard()
    await user.click(await screen.findByRole('button', { name: 'Check now' }))

    expect(
      await screen.findByText(/redeployed with a newer image/),
    ).toBeInTheDocument()
  })

  it('toasts after enabling', async () => {
    fetchMock.mockImplementation(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        Promise.resolve(json({ enabled: init?.method === 'PUT' })),
    )

    const user = userEvent.setup()
    renderCard()
    await user.click(
      await screen.findByRole('switch', { name: 'Image auto-update enabled' }),
    )

    await waitFor(() => {
      expect(toast.add).toHaveBeenCalledWith({
        title: 'Image auto-update enabled.',
        type: 'success',
      })
    })
  })
})
