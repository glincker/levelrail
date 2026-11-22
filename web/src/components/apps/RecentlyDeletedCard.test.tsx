import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import lifecycle from '../../locales/en/lifecycle.json'
import { RecentlyDeletedCard } from './RecentlyDeletedCard'
import { createTestQueryClient } from '@/test/testQueryClient'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['lifecycle'],
  defaultNS: 'lifecycle',
  resources: { en: { lifecycle } },
  interpolation: { escapeValue: false },
})

afterEach(() => vi.unstubAllGlobals())

describe('RecentlyDeletedCard', () => {
  it('lists a deleted app and restores it', async () => {
    const fetchFn = vi.fn((input: string, init?: { method?: string }) =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve(
            init?.method === 'POST'
              ? {}
              : [
                  {
                    kind: 'app',
                    name: 'web',
                    deleted_at: '2026-11-20T10:00:00Z',
                    expires_at: '2026-11-27T10:00:00Z',
                  },
                ],
          ),
        url: input,
      }),
    )
    vi.stubGlobal('fetch', fetchFn)
    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <I18nextProvider i18n={i18n}>
          <RecentlyDeletedCard />
        </I18nextProvider>
      </QueryClientProvider>,
    )
    await userEvent.click(
      await screen.findByRole('button', { name: 'Restore' }),
    )
    await waitFor(() => {
      expect(fetchFn).toHaveBeenCalledWith(
        '/api/v1/recently-deleted/app/web/restore',
        { method: 'POST' },
      )
    })
  })
})
