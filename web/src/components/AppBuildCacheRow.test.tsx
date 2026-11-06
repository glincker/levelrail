import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AppBuildCacheRow } from './AppBuildCacheRow'
import en from '../locales/en/appBuildCache.json'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['appBuildCache'],
  defaultNS: 'appBuildCache',
  resources: { en: { appBuildCache: en } },
  interpolation: { escapeValue: false },
})

function mount() {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <AppBuildCacheRow appName="web" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

function json(body: unknown) {
  return Promise.resolve(new Response(JSON.stringify(body), { status: 200 }))
}

describe('AppBuildCacheRow', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('shows the empty state when no cache exists', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() =>
        json({
          cache_id: 'web',
          size_bytes: 0,
          record_count: 0,
          in_use: false,
        }),
      ),
    )
    mount()
    expect(await screen.findByText(/first build fills it/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Clear cache' })).toBeNull()
  })

  it('confirms before clearing and sends DELETE', async () => {
    const fetchMock = vi.fn((_url: string, init?: RequestInit) =>
      init?.method === 'DELETE'
        ? json({ cache_id: 'web', freed_bytes: 2048, removed_count: 3 })
        : json({
            cache_id: 'web',
            size_bytes: 2048,
            record_count: 3,
            in_use: false,
            last_used_at: new Date().toISOString(),
          }),
    )
    vi.stubGlobal('fetch', fetchMock)
    mount()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Clear cache' }),
    )
    expect(screen.getByText(/Clear this app's build cache/)).toBeTruthy()
    const buttons = screen.getAllByRole('button', { name: 'Clear cache' })
    await userEvent.click(buttons[buttons.length - 1])
    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some(([, init]) => init?.method === 'DELETE'),
      ).toBe(true),
    )
  })
})
