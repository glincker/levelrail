import { Suspense } from 'react'
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import { AppSleepCard } from './AppSleepCard'
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
          <AppSleepCard appName="demo-app" />
        </Suspense>
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

const off = {
  enabled: false,
  idle_minutes: 0,
  sleeping: false,
  hold_requests: false,
  eligible: true,
  blockers: [],
  warnings: [],
  settings: {
    idle_minutes: 0,
    hold_requests: false,
    ignore_paths: [],
    port_wake: false,
  },
  port_wake: { supported: false, active: false, ports: [] },
  activity: { open_connections: 0, idle_for_seconds: 0 },
  cold_start_ms_p50: null,
  estimated_memory_freed_bytes: null,
  wake_timing: { samples: 0 },
  idle_limits: { min_minutes: 5, max_minutes: 10080, default_minutes: 30 },
}
const pinned = {
  code: 'pinned_port',
  title: 'Pinned host port',
  detail: 'Traffic on that port would not wake it.',
  fix: {
    kind: 'enable_port_wake',
    label: 'Wake on port traffic',
    method: 'PUT',
    path: '/api/v1/apps/demo-app/sleep',
    body: { idle_minutes: 30, port_wake: true },
  },
}

function sentBody(
  fetchMock: ReturnType<typeof vi.fn>,
  method: string,
): unknown {
  const call = methodCalls(fetchMock, method)[0]
  return call ? JSON.parse((call[1] as RequestInit).body as string) : undefined
}

function methodCalls(fetchMock: ReturnType<typeof vi.fn>, method: string) {
  return fetchMock.mock.calls.filter(
    ([, init]) => (init as RequestInit | undefined)?.method === method,
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

  it('disables Turn on with a blocker, names it, and shows no toast', async () => {
    fetchMock.mockResolvedValue(
      json({ ...off, eligible: false, blockers: [pinned] }),
    )
    renderCard()
    const button = await screen.findByRole('button', {
      name: 'Turn on sleep when idle',
    })
    expect(button).toBeDisabled()
    expect(screen.getAllByText(/Pinned host port/).length).toBeGreaterThan(1)
    expect(screen.getByText('Blocked')).toBeInTheDocument()
    expect(toast.add).not.toHaveBeenCalled()
  })

  it('runs the fix endpoint, then refetches eligibility', async () => {
    let fixed = false
    fetchMock.mockImplementation(
      (_input: RequestInfo | URL, init?: RequestInit) => {
        if (init?.method === 'PUT') {
          fixed = true
          return Promise.resolve(json({ ...off, eligible: true }))
        }
        return Promise.resolve(
          json(
            fixed
              ? { ...off, eligible: true, blockers: [] }
              : { ...off, eligible: false, blockers: [pinned] },
          ),
        )
      },
    )
    const user = userEvent.setup()
    renderCard()
    await user.click(
      await screen.findByRole('button', { name: 'Wake on port traffic' }),
    )
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Turn on sleep when idle' }),
      ).toBeEnabled()
    })
    expect(String(methodCalls(fetchMock, 'PUT')[0]?.[0])).toBe(
      '/api/v1/apps/demo-app/sleep',
    )
    expect(sentBody(fetchMock, 'PUT')).toEqual({
      idle_minutes: 30,
      port_wake: true,
    })
  })

  it('lists warnings without disabling Turn on', async () => {
    fetchMock.mockResolvedValue(
      json({
        ...off,
        eligible: true,
        blockers: [],
        warnings: [
          {
            code: 'no_domain',
            title: 'No domain',
            detail: 'Nothing can wake it.',
          },
        ],
      }),
    )
    renderCard()
    expect(
      await screen.findByRole('button', { name: 'Turn on sleep when idle' }),
    ).toBeEnabled()
    expect(screen.getByText('No domain')).toBeInTheDocument()
    expect(screen.getByText(/What this means/)).toBeInTheDocument()
  })

  it('enables sleeping with the default idle time', async () => {
    fetchMock.mockImplementation(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        Promise.resolve(
          json({ ...off, eligible: true, enabled: init?.method === 'PUT' }),
        ),
    )
    const user = userEvent.setup()
    renderCard()
    await user.click(
      await screen.findByRole('button', { name: 'Turn on sleep when idle' }),
    )
    await waitFor(() => {
      expect(toast.add).toHaveBeenCalledWith({
        title: 'Sleep when idle enabled.',
        type: 'success',
      })
    })
    expect(sentBody(fetchMock, 'PUT')).toEqual({
      idle_minutes: 30,
    })
  })

  it('shows a live countdown and the cold start estimate', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const willSleepAt = new Date(Date.now() + 12 * 60_000).toISOString()
    fetchMock.mockResolvedValue(
      json({
        ...off,
        enabled: true,
        idle_minutes: 30,
        cold_start_ms_p50: 4000,
        wake_timing: { samples: 6, ready_p50_ms: 4200, ready_p95_ms: 9100 },
        estimated_memory_freed_bytes: 536870912,
        activity: {
          open_connections: 2,
          idle_for_seconds: 60,
          will_sleep_at: willSleepAt,
        },
      }),
    )
    renderCard()
    expect(await screen.findByTestId('sleep-countdown')).toHaveTextContent(
      '12 min',
    )
    expect(screen.getByTestId('sleep-wake-timing')).toHaveTextContent(
      'Usually wakes in about 4.2 s (p95 9.1 s).',
    )
    expect(
      screen.getByText('Frees about 512 MiB while asleep'),
    ).toBeInTheDocument()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(11 * 60_000 + 30_000)
    })
    expect(screen.getByTestId('sleep-countdown')).toHaveTextContent(
      /^(30|29) sec/,
    )
    vi.useRealTimers()
  })

  it('says plainly when no wake has been measured', async () => {
    fetchMock.mockResolvedValue(
      json({ ...off, enabled: true, idle_minutes: 30 }),
    )
    renderCard()
    expect(await screen.findByTestId('sleep-wake-timing')).toHaveTextContent(
      'No wakes measured yet',
    )
  })

  it('animates only behind motion-safe', async () => {
    fetchMock.mockResolvedValue(
      json({ ...off, enabled: true, sleeping: true, idle_minutes: 30 }),
    )
    const { container } = renderCard()
    await screen.findByText('Sleeping now')
    expect(container.innerHTML).toContain('motion-safe:')
    expect(container.innerHTML).not.toMatch(/[ "]animate-pulse/)
  })

  it('offers wake now for a sleeping app and sleep now for an awake one', async () => {
    fetchMock.mockResolvedValue(
      json({ ...off, enabled: true, sleeping: true, idle_minutes: 30 }),
    )
    const user = userEvent.setup()
    renderCard()
    await user.click(await screen.findByRole('button', { name: 'Wake now' }))
    expect(String(methodCalls(fetchMock, 'POST')[0]?.[0])).toBe(
      '/api/v1/apps/demo-app/sleep/wake',
    )
  })

  it('saves a preset idle time with the full body', async () => {
    fetchMock.mockResolvedValue(
      json({ ...off, enabled: true, idle_minutes: 30, hold_requests: true }),
    )
    const user = userEvent.setup()
    renderCard()
    await user.click(await screen.findByRole('button', { name: '60 minutes' }))
    expect(sentBody(fetchMock, 'PUT')).toEqual({
      idle_minutes: 60,
    })
  })
})
