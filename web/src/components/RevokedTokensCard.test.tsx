import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import type { ReactNode } from 'react'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { RevokedTokensCard } from './RevokedTokensCard'
import settingsEn from '../locales/en/settings.json'
import runtimeEn from '../locales/en/runtimeSettings.json'
import commonEn from '../locales/en/common.json'
import type { TokenResource } from '../types/token'

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => <a href="/x">{children}</a>,
}))
vi.mock('../hooks/useIsRoot', () => ({ useIsRoot: () => true }))
vi.mock('../queries/runtimeSettings', () => ({
  useRuntimeSettings: () => ({
    data: {
      settings: [
        {
          key: 'retention.revoked_token_days',
          value: 90,
          allowed_values: [0, 30, 60, 90, 365],
        },
      ],
    },
  }),
  useSetRuntimeSetting: () => ({ mutate: vi.fn(), isPending: false }),
}))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings', 'runtimeSettings', 'common'],
  defaultNS: 'settings',
  resources: {
    en: { settings: settingsEn, runtimeSettings: runtimeEn, common: commonEn },
  },
  interpolation: { escapeValue: false },
})

function token(id: string, revoked: boolean): TokenResource {
  return {
    id,
    name: `tok-${id}`,
    abilities: ['read'],
    created_at: new Date(Date.now() - 86_400_000).toISOString(),
    ...(revoked ? { revoked_at: new Date().toISOString() } : {}),
  }
}

function renderCard(tokens: TokenResource[]) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <RevokedTokensCard tokens={tokens} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('RevokedTokensCard', () => {
  it('shows the empty state when nothing is revoked', () => {
    renderCard([token('a', false)])
    fireEvent.click(screen.getByText('Revoked tokens'))
    expect(screen.getByText(/No revoked tokens/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Clean up now' })).toBeDisabled()
  })

  it('confirms with the count, then cleans up every revoked token', async () => {
    const fetchMock = vi.fn(() =>
      Promise.resolve(
        new Response(JSON.stringify({ deleted: 2 }), { status: 200 }),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderCard([token('a', true), token('b', true), token('c', false)])
    fireEvent.click(screen.getByText('Revoked tokens'))
    fireEvent.click(screen.getByRole('button', { name: 'Clean up now' }))
    expect(
      await screen.findByText('2 revoked tokens will be removed for good.'),
    ).toBeInTheDocument()
    fireEvent.click(
      screen.getByRole('button', { name: 'Remove revoked tokens' }),
    )
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled()
    })
    const call = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(call[0]).toBe('/api/v1/auth/tokens/cleanup')
    expect(call[1].body).toBe('{"older_than_days":0}')
  })
})
