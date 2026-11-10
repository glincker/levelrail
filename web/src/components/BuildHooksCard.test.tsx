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

  it('shows the allowlist and saves an edited one with PATCH', async () => {
    fetchMock.mockImplementation(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        Promise.resolve(
          init?.method === 'PATCH'
            ? json({ ...existing, allowed_cidrs: ['10.0.0.0/8'] })
            : json([
                {
                  ...existing,
                  allowed_cidrs: ['203.0.113.0/24'],
                  last_used_at: '2026-10-03T10:00:00Z',
                  last_refusal: 'refused: source address not allowed',
                },
              ]),
        ),
    )
    const user = userEvent.setup()
    renderCard()
    expect(
      await screen.findByText(/Only 203\.0\.113\.0\/24 may call it/),
    ).toBeInTheDocument()
    expect(
      screen.getByText(/Last refused call: refused: source address/),
    ).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Allowlist' }))
    const [field] = screen.getAllByLabelText('Allowed sources (optional)')
    if (!field) throw new Error('allowlist field missing')
    expect(field).toHaveValue('203.0.113.0/24')
    await user.clear(field)
    expect(
      screen.getByRole('button', { name: 'Save allowlist' }),
    ).toBeDisabled()
    await user.type(field, '10.0.0.0/8, 192.0.2.1')
    await user.click(screen.getByRole('button', { name: 'Save allowlist' }))
    await waitFor(() => {
      const patch = fetchMock.mock.calls.find(
        ([, init]) => (init as RequestInit | undefined)?.method === 'PATCH',
      )
      expect(patch?.[0]).toBe('/api/v1/apps/demo-app/build-hooks/bh_1')
      expect(JSON.parse((patch?.[1] as RequestInit).body as string)).toEqual({
        allowed_cidrs: ['10.0.0.0/8', '192.0.2.1'],
      })
    })
  })

  it('clears the allowlist only through the explicit allow any toggle', async () => {
    fetchMock.mockImplementation(
      (_input: RequestInfo | URL, init?: RequestInit) =>
        Promise.resolve(
          init?.method === 'PATCH'
            ? json(existing)
            : json([{ ...existing, allowed_cidrs: ['203.0.113.0/24'] }]),
        ),
    )
    const user = userEvent.setup()
    renderCard()
    await user.click(await screen.findByRole('button', { name: 'Allowlist' }))
    await user.click(screen.getByRole('checkbox', { name: 'Allow any source' }))
    await user.click(screen.getByRole('button', { name: 'Save allowlist' }))
    await waitFor(() => {
      const patch = fetchMock.mock.calls.find(
        ([, init]) => (init as RequestInit | undefined)?.method === 'PATCH',
      )
      expect(JSON.parse((patch?.[1] as RequestInit).body as string)).toEqual({
        allow_any: true,
      })
    })
  })
})
