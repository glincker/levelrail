import { render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import tokensNoticesEn from '../../locales/en/tokensNotices.json'
import commonEn from '../../locales/en/common.json'
import { createTestQueryClient } from '@/test/testQueryClient'
import { TokensNotices } from './TokensNotices'
import type { TokenResource } from '../../types/token'

vi.mock('../RevokeTokenDialog', () => ({
  RevokeTokenDialog: ({ token }: { token: TokenResource }) => (
    <button type="button">Revoke {token.name}</button>
  ),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['tokensNotices', 'common'],
  defaultNS: 'common',
  resources: { en: { tokensNotices: tokensNoticesEn, common: commonEn } },
})

const active = [
  {
    id: 't1',
    name: 'ci-deploy',
    abilities: ['read'],
    created_at: '2026-01-01T00:00:00Z',
  },
] as TokenResource[]

function renderNotices(items: unknown[]) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({ alert_days: items.length ? 30 : 0, items }),
      } as unknown as Response),
    ),
  )
  return render(
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={createTestQueryClient()}>
        <TokensNotices expiringSoon={0} longLivedRoot={0} active={active} />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('TokensNotices', () => {
  it('renders nothing when the endpoint returns no rows', async () => {
    const { container } = renderNotices([])
    await waitFor(() => {
      expect(fetch).toHaveBeenCalled()
    })
    expect(container).toBeEmptyDOMElement()
  })

  it('lists unused tokens with a revoke action', async () => {
    renderNotices([
      {
        id: 't1',
        name: 'ci-deploy',
        abilities: ['read'],
        created_at: '2026-01-01T00:00:00Z',
      },
    ])
    expect(await screen.findByText('ci-deploy')).toBeVisible()
    expect(
      screen.getByRole('button', { name: 'Revoke ci-deploy' }),
    ).toBeVisible()
    expect(screen.getByText('never used')).toBeVisible()
  })
})
