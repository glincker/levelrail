import { Suspense } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { BuildHooksCard } from './BuildHooksCard'
import deploysEn from '../locales/en/deploys.json'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@/components/ui/toast', () => ({
  toast: { add: vi.fn() },
}))

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
          <BuildHooksCard appName="demo-app" />
        </Suspense>
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

const existing = {
  id: 'bh_1',
  name: 'cms publish',
  created_at: '2026-10-01T00:00:00Z',
  last_triggered_at: '2026-10-02T10:00:00Z',
  last_result: 'deployed',
  trigger_count: 3,
}

describe('BuildHooksCard', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows the empty state', async () => {
    fetchMock.mockResolvedValue(json([]))
    renderCard()
    expect(await screen.findByText(/No build hooks yet/)).toBeInTheDocument()
  })

  it('lists hooks with last triggered info and never the secret', async () => {
    fetchMock.mockResolvedValue(json([existing]))
    renderCard()
    expect(await screen.findByText('cms publish')).toBeInTheDocument()
    expect(screen.getByText(/3 total/)).toBeInTheDocument()
    expect(screen.queryByLabelText('Build hook URL')).not.toBeInTheDocument()
  })

  it('reveals the URL once after creating a hook', async () => {
    fetchMock.mockImplementation(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        Promise.resolve(
          init?.method === 'POST'
            ? json(
                {
                  ...existing,
                  path: '/api/v1/hooks/deploy/bh_1/tok',
                  token: 'tok',
                },
                201,
              )
            : json([]),
        ),
    )
    const user = userEvent.setup()
    renderCard()
    await user.type(await screen.findByLabelText('Hook name'), 'cms publish')
    await user.click(screen.getByRole('button', { name: 'Create hook' }))

    const url = await screen.findByLabelText('Build hook URL')
    expect(url).toHaveValue(
      `${window.location.origin}/api/v1/hooks/deploy/bh_1/tok`,
    )
    const post = fetchMock.mock.calls.find(
      ([, init]) => (init as RequestInit | undefined)?.method === 'POST',
    )
    expect(JSON.parse((post?.[1] as RequestInit).body as string)).toEqual({
      name: 'cms publish',
      branch: '',
    })
  })

  it('needs a second click to revoke', async () => {
    fetchMock.mockImplementation(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        Promise.resolve(
          init?.method === 'DELETE'
            ? new Response(null, { status: 204 })
            : json([existing]),
        ),
    )
    const user = userEvent.setup()
    renderCard()
    await user.click(await screen.findByRole('button', { name: 'Revoke' }))
    expect(
      fetchMock.mock.calls.some(
        ([, init]) => (init as RequestInit | undefined)?.method === 'DELETE',
      ),
    ).toBe(false)
    await user.click(screen.getByRole('button', { name: 'Confirm revoke' }))
    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(
          ([, init]) => (init as RequestInit | undefined)?.method === 'DELETE',
        ),
      ).toBe(true),
    )
  })
})
