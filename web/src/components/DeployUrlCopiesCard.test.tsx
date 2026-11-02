import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { DeployUrlCopiesCard } from './DeployUrlCopiesCard'
import type { DeployUrlCopies } from '../queries/deployUrlCopies'
import settingsEn from '../locales/en/settings.json'
import commonEn from '../locales/en/common.json'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings', 'common'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

function jsonResponse(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: () => Promise.resolve(body),
  } as Response
}

describe('DeployUrlCopiesCard', () => {
  let state: DeployUrlCopies
  let puts: unknown[]

  beforeEach(() => {
    puts = []
    state = {
      mode: 'private',
      source: 'env',
      default: 'private',
      modes: ['off', 'private', 'public'],
    }
    vi.stubGlobal(
      'fetch',
      vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
        if (init?.method === 'PUT') {
          const body = JSON.parse(init.body as string) as { mode: string }
          puts.push(body)
          state = {
            ...state,
            mode:
              body.mode === ''
                ? 'private'
                : (body.mode as DeployUrlCopies['mode']),
            source: body.mode === '' ? 'default' : 'stored',
          }
        }
        return Promise.resolve(jsonResponse(state))
      }),
    )
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  function renderCard() {
    render(
      <I18nextProvider i18n={testI18n}>
        <QueryClientProvider client={createTestQueryClient()}>
          <DeployUrlCopiesCard />
        </QueryClientProvider>
      </I18nextProvider>,
    )
  }

  it('renders the effective mode and its source', async () => {
    renderCard()
    expect(await screen.findByText('From environment')).toBeInTheDocument()
    expect(screen.getByRole('radio', { name: /Private/ })).toBeChecked()
  })

  it('saves a change to off through the API', async () => {
    const user = userEvent.setup()
    renderCard()
    await user.click(await screen.findByRole('radio', { name: /Off/ }))
    await waitFor(() => expect(puts).toEqual([{ mode: 'off' }]))
    expect(await screen.findByText('Saved here')).toBeInTheDocument()
  })

  it('asks before switching to public', async () => {
    const user = userEvent.setup()
    renderCard()
    await user.click(await screen.findByRole('radio', { name: /Public/ }))
    expect(puts).toEqual([])
    await user.click(await screen.findByRole('button', { name: 'Make public' }))
    await waitFor(() => expect(puts).toEqual([{ mode: 'public' }]))
  })
})
