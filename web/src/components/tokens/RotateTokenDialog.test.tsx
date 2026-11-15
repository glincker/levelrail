import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { RotateTokenDialog } from './RotateTokenDialog'
import tokenExpiryEn from '../../locales/en/tokenExpiry.json'
import { BrandContext } from '../../lib/brandContext'
import type { Brand } from '../../types/brand'
import type { TokenResource } from '../../types/token'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['tokenExpiry'],
  defaultNS: 'tokenExpiry',
  resources: { en: { tokenExpiry: tokenExpiryEn } },
  interpolation: { escapeValue: false },
})

const tok: TokenResource = {
  id: 'tok_1',
  name: 'ci-deploy',
  abilities: ['read'],
  created_at: '2026-11-01T00:00:00Z',
}

function mockFetch(status: number, body: unknown) {
  const fn = vi.fn(() =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { 'Content-Type': 'application/json' },
      }),
    ),
  )
  vi.stubGlobal('fetch', fn)
  return fn
}

function setup(onClose = vi.fn()) {
  const client = new QueryClient({
    defaultOptions: { mutations: { retry: false } },
  })
  const spy = vi.spyOn(client, 'invalidateQueries')
  const view = render(
    <QueryClientProvider client={client}>
      <BrandContext.Provider value={{ BinaryName: 'levelrail-cli' } as Brand}>
        <I18nextProvider i18n={testI18n}>
          <RotateTokenDialog token={tok} grace="24h" onClose={onClose} />
        </I18nextProvider>
      </BrandContext.Provider>
    </QueryClientProvider>,
  )
  return { onClose, spy, view }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('RotateTokenDialog', () => {
  it('explains the grace period and shows the CLI hint', () => {
    setup()
    expect(screen.getByText(/keeps working for 24h/)).toBeInTheDocument()
    expect(
      screen.getByText('levelrail-cli tokens rotate ci-deploy'),
    ).toBeInTheDocument()
  })

  it('reveals the secret once, invalidates, and clears it on close', async () => {
    const user = userEvent.setup()
    mockFetch(201, {
      ...tok,
      token: 'lr_secret_value',
      replaces: 'tok_1',
      previous_valid_until: '2026-11-16T12:00:00Z',
    })
    const { onClose, spy } = setup()
    await user.click(screen.getByRole('button', { name: 'Rotate token' }))
    expect(await screen.findByText('lr_secret_value')).toBeInTheDocument()
    expect(screen.getByText(/shown once/)).toBeInTheDocument()
    expect(spy).toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Done' }))
    expect(onClose).toHaveBeenCalled()
    expect(screen.queryByText('lr_secret_value')).not.toBeInTheDocument()
  })

  it('says a platform token cannot be rotated on 403', async () => {
    const user = userEvent.setup()
    mockFetch(403, { error: 'platform tokens rotate themselves' })
    setup()
    await user.click(screen.getByRole('button', { name: 'Rotate token' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Platform tokens rotate themselves',
    )
  })

  it('says the token is already revoked on 409 and still refreshes', async () => {
    const user = userEvent.setup()
    mockFetch(409, { error: 'token is already revoked' })
    const { spy } = setup()
    await user.click(screen.getByRole('button', { name: 'Rotate token' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'already revoked',
    )
    await waitFor(() => {
      expect(spy).toHaveBeenCalled()
    })
  })

  it('reports a network failure honestly', async () => {
    const user = userEvent.setup()
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.reject(new TypeError('Failed to fetch'))),
    )
    setup()
    await user.click(screen.getByRole('button', { name: 'Rotate token' }))
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Could not reach the server',
    )
  })
})
