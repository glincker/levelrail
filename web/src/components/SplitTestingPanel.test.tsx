import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import { SplitTestingPanel } from './SplitTestingPanel'
import splitEn from '../locales/en/splitTesting.json'
import type { SplitTest } from '../queries/splitTesting'

vi.mock('@/components/ui/toast', () => ({
  toast: { add: vi.fn() },
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['splitTesting'],
  defaultNS: 'splitTesting',
  resources: { en: { splitTesting: splitEn } },
  interpolation: { escapeValue: false },
})

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

const running: SplitTest = {
  active: true,
  supported: true,
  test_id: 't1',
  status: 'running',
  cookie_name: 'lr_split',
  cookie_ttl_secs: 7200,
  override_param: 'lr_variant',
  total_requests: 40,
  variants: [
    {
      id: 't1-p',
      label: 'Production',
      kind: 'production',
      weight: 70,
      status: 'live',
      requests: 30,
      share: 0.75,
      qa_link: 'https://app.example.com/?lr_variant=t1-p',
    },
    {
      id: 't1-1',
      label: 'Redesign',
      kind: 'branch',
      ref: 'redesign',
      weight: 30,
      status: 'ready',
      requests: 10,
      share: 0.25,
      qa_link: 'https://app.example.com/?lr_variant=t1-1',
    },
  ],
  hourly: [
    { bucket: '2026-10-20T10:00:00Z', counts: { 't1-p': 30, 't1-1': 10 } },
  ],
}

function renderPanel() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={queryClient}>
        <SplitTestingPanel appName="demo" />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

describe('SplitTestingPanel', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    vi.mocked(toast.add).mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows per-variant request counts and the QA link action', async () => {
    fetchMock.mockResolvedValue(json(running))
    renderPanel()
    expect((await screen.findAllByText('Redesign')).length).toBeGreaterThan(0)
    expect(screen.getByText('(75%)')).toBeInTheDocument()
    expect(
      screen.getAllByRole('button', { name: 'Copy QA link' }),
    ).toHaveLength(2)
  })

  it('saves edited weights with production taking the remainder', async () => {
    fetchMock.mockImplementation(() => Promise.resolve(json(running)))
    renderPanel()
    const slider = await screen.findByLabelText(
      'Share of visitors for Redesign',
    )
    const save = screen.getByRole('button', { name: 'Save weights' })
    expect(save).toBeDisabled()
    const user = userEvent.setup()
    fireEvent.change(slider, { target: { value: '31' } })
    await waitFor(() => expect(save).toBeEnabled())
    expect(screen.getByText('Production keeps 69%')).toBeInTheDocument()
    await user.click(save)
    await waitFor(() => {
      const put = fetchMock.mock.calls.find(
        ([, init]) => (init as RequestInit | undefined)?.method === 'PUT',
      )
      expect(put).toBeDefined()
      expect(JSON.parse((put?.[1] as RequestInit).body as string)).toEqual({
        weights: { 't1-p': 69, 't1-1': 31 },
      })
    })
  })

  it('says static sites are not supported', async () => {
    fetchMock.mockResolvedValue(
      json({
        active: false,
        supported: false,
        total_requests: 0,
        variants: [],
      }),
    )
    renderPanel()
    expect(
      await screen.findByText('Split testing is not available for this app'),
    ).toBeInTheDocument()
  })

  it('offers the start form when no test is running', async () => {
    fetchMock.mockImplementation((input: RequestInfo | URL) =>
      Promise.resolve(
        (input instanceof Request ? input.url : String(input)).includes(
          'branch-deploys',
        )
          ? json({ settings: {}, deploys: [] })
          : json({
              active: false,
              supported: true,
              total_requests: 0,
              variants: [],
            }),
      ),
    )
    renderPanel()
    expect(
      await screen.findByText('No split test is running'),
    ).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Start test' })).toBeDisabled()
  })
})
