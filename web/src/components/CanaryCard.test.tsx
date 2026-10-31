import { Suspense } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import { CanaryCard } from './CanaryCard'
import deploysEn from '../locales/en/deploys.json'
import type { Brand } from '../types/brand'
import { createTestQueryClient } from '@/test/testQueryClient'

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
  const queryClient = createTestQueryClient()
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
    await user.click(await screen.findByRole('button', { name: 'Promote now' }))

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

  it('shows stage progress and the live comparison for a staged canary', async () => {
    fetchMock.mockResolvedValue(
      json({
        active: true,
        image: 'nginx:2',
        weight: 25,
        staged: true,
        stages: [10, 25, 50, 100],
        stage: 2,
        progress: {
          stage: 2,
          stage_count: 4,
          elapsed_secs: 30,
          interval_secs: 300,
          canary_requests: 120,
          canary_error_rate: 0.05,
          canary_p95_ms: 240,
          stable_requests: 900,
          stable_error_rate: 0.001,
          stable_p95_ms: 90,
          next: 'observing stage',
          breaching: true,
        },
      }),
    )

    renderCard()
    expect(await screen.findByText('Stage 2 of 4')).toBeInTheDocument()
    expect(screen.getByText('5.0%')).toBeInTheDocument()
    expect(screen.getByText('240 ms')).toBeInTheDocument()
    expect(
      screen.getByText('Breach: observing stage. Rolling back.'),
    ).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: 'Promote now' }),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Abort' })).toBeInTheDocument()
  })

  it('starts a staged canary when stages are given', async () => {
    fetchMock.mockImplementation(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        Promise.resolve(
          init?.method === 'POST'
            ? json({
                active: true,
                image: 'nginx:2',
                weight: 10,
                staged: true,
                stages: [10, 50],
                stage: 1,
              })
            : json({ active: false, weight: 0 }),
        ),
    )
    const user = userEvent.setup()
    renderCard()
    await user.type(await screen.findByLabelText('Canary image'), 'nginx:2')
    await user.type(screen.getByLabelText('Stages % (optional)'), '10,50')
    await user.click(screen.getByRole('button', { name: 'Start canary' }))

    await waitFor(() => {
      const post = fetchMock.mock.calls.find(
        ([, init]) => (init as RequestInit | undefined)?.method === 'POST',
      )
      expect(JSON.parse((post?.[1] as RequestInit).body as string)).toEqual({
        image: 'nginx:2',
        stages: [10, 50],
      })
    })
  })
})
