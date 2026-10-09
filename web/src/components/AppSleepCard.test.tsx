import { Suspense } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import { AppSleepCard } from './AppSleepCard'
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
          <AppSleepCard appName="demo-app" />
        </Suspense>
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('AppSleepCard', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    vi.mocked(toast.add).mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('enables sleeping with the chosen idle time', async () => {
    fetchMock.mockImplementation(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        Promise.resolve(
          init?.method === 'PUT'
            ? json({
                enabled: true,
                idle_minutes: 30,
                sleeping: false,
                hold_requests: false,
              })
            : json({
                enabled: false,
                idle_minutes: 0,
                sleeping: false,
                hold_requests: false,
              }),
        ),
    )

    const user = userEvent.setup()
    renderCard()
    await user.click(
      await screen.findByRole('switch', { name: 'Sleep when idle enabled' }),
    )

    await waitFor(() => {
      expect(toast.add).toHaveBeenCalledWith({
        title: 'Sleep when idle enabled.',
        type: 'success',
      })
    })
    const put = fetchMock.mock.calls.find(
      ([, init]) => (init as RequestInit | undefined)?.method === 'PUT',
    )
    expect(JSON.parse((put?.[1] as RequestInit).body as string)).toEqual({
      idle_minutes: 30,
    })
  })

  it('turns function mode on for an enabled app', async () => {
    fetchMock.mockImplementation(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        Promise.resolve(
          json({
            enabled: true,
            idle_minutes: 30,
            sleeping: false,
            hold_requests: init?.method === 'PUT',
          }),
        ),
    )

    const user = userEvent.setup()
    renderCard()
    await user.click(
      await screen.findByRole('switch', { name: 'Hold requests while waking' }),
    )

    await waitFor(() => {
      expect(toast.add).toHaveBeenCalledWith({
        title: 'Function mode updated.',
        type: 'success',
      })
    })
    const put = fetchMock.mock.calls.find(
      ([, init]) => (init as RequestInit | undefined)?.method === 'PUT',
    )
    expect(JSON.parse((put?.[1] as RequestInit).body as string)).toEqual({
      idle_minutes: 30,
      hold_requests: true,
    })
  })

  it('offers wake now for a sleeping app', async () => {
    fetchMock.mockImplementation(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        Promise.resolve(
          init?.method === 'POST'
            ? json({
                enabled: true,
                idle_minutes: 30,
                sleeping: false,
                hold_requests: false,
              })
            : json({
                enabled: true,
                idle_minutes: 30,
                sleeping: true,
                hold_requests: false,
              }),
        ),
    )

    const user = userEvent.setup()
    renderCard()
    expect(await screen.findByText('This app is asleep.')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Wake now' }))

    await waitFor(() => {
      expect(screen.queryByText('This app is asleep.')).not.toBeInTheDocument()
    })
  })
})
