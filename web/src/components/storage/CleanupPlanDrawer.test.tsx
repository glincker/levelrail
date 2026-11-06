import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { TooltipProvider } from '@/components/ui/tooltip'
import commonEn from '../../locales/en/common.json'
import cleanupEn from '../../locales/en/storageCleanup.json'
import { CleanupPlanDrawer } from './CleanupPlanDrawer'

const granted = vi.hoisted(() => ({ admin: true }))

vi.mock('@/hooks/useAbilities', () => ({
  useAbilities: () => ({ role: null, can: () => granted.admin }),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'storageCleanup'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, storageCleanup: cleanupEn } },
  interpolation: { escapeValue: false },
})

const plan = {
  id: 'plan-1',
  started_at: '2026-11-05T10:00:00Z',
  trigger: 'manual',
  dry_run: true,
  free_percent: 30,
  pressure: false,
  reclaimed_bytes: 0,
  stages: [
    {
      stage: 'images',
      status: 'ran',
      reclaimed_bytes: 0,
      items: [
        {
          name: 'img-a',
          detail: 'dangling',
          size_bytes: 2048,
          action: 'would_remove',
        },
      ],
    },
    {
      stage: 'volumes',
      status: 'ran',
      reclaimed_bytes: 0,
      items: [{ name: 'db-main', size_bytes: 4096, action: 'kept' }],
    },
  ],
}

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

function wrap(children: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return (
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>
        <TooltipProvider>{children}</TooltipProvider>
      </I18nextProvider>
    </QueryClientProvider>
  )
}

describe('CleanupPlanDrawer', () => {
  const fetchMock =
    vi.fn<(url: string, init?: RequestInit) => Promise<Response>>()

  beforeEach(() => {
    granted.admin = true
    fetchMock.mockReset()
    fetchMock.mockImplementation((url: string, init?: RequestInit) => {
      if (url.includes('/janitor/run') && init?.body) {
        return Promise.resolve(
          json({
            ...plan,
            id: 'run-2',
            dry_run: url.includes('dry_run=true'),
            reclaimed_bytes: 2048,
            ignored_items: [
              {
                stage: 'images',
                name: 'img-z',
                reason: 'no matching cleanup candidate',
              },
            ],
          }),
        )
      }
      return Promise.resolve(json(plan))
    })
    vi.stubGlobal('fetch', fetchMock)
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('lists the dry run with a locked database volume', async () => {
    render(wrap(<CleanupPlanDrawer open onOpenChange={() => {}} />))
    expect(await screen.findByText('img-a')).toBeInTheDocument()
    expect(screen.getByText('db-main')).toBeInTheDocument()
    expect(screen.getByText(cleanupEn.locked.database)).toBeInTheDocument()
    expect(screen.queryByRole('checkbox', { name: /db-main/ })).toBeNull()
    expect(screen.getByText('2.0 KiB selected in 1 items')).toBeInTheDocument()
  })

  it('applies the allowlist only after the confirmation line', async () => {
    render(wrap(<CleanupPlanDrawer open onOpenChange={() => {}} />))
    await screen.findByText('img-a')
    fireEvent.click(screen.getByRole('button', { name: 'Apply' }))
    expect(
      screen.getByText(
        'Remove 2.0 KiB in 1 items now? Deleted data cannot be restored.',
      ),
    ).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Yes, delete' }))
    expect(
      await screen.findByText('Done: 2.0 KiB reclaimed.'),
    ).toBeInTheDocument()
    expect(screen.getByText(/img-z \(Images\)/)).toBeInTheDocument()
    const call = fetchMock.mock.calls.find(
      ([u, i]) => String(u).includes('dry_run=false') && i?.body,
    )
    const body = call?.[1]?.body
    expect(typeof body === 'string' ? JSON.parse(body) : null).toEqual({
      items: [{ stage: 'images', names: ['img-a'] }],
    })
  })

  it('keeps Apply blocked for a non-admin but shows the plan', async () => {
    granted.admin = false
    render(wrap(<CleanupPlanDrawer open onOpenChange={() => {}} />))
    expect(await screen.findByText('img-a')).toBeInTheDocument()
    const apply = screen.getByRole('button', { name: 'Apply' })
    expect(apply).toHaveAttribute('aria-disabled', 'true')
    fireEvent.click(apply)
    expect(screen.queryByText(/cannot be restored/)).toBeNull()
  })

  it('shows the empty state', async () => {
    fetchMock.mockImplementation(() =>
      Promise.resolve(json({ ...plan, stages: [] })),
    )
    render(wrap(<CleanupPlanDrawer open onOpenChange={() => {}} />))
    await waitFor(() =>
      expect(screen.getByText('Nothing needs removing.')).toBeInTheDocument(),
    )
  })
})
