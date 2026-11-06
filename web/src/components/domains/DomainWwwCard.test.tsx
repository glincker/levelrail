import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import domainsEn from '../../locales/en/domains.json'
import { createTestQueryClient } from '@/test/testQueryClient'
import { DomainWwwCard } from './DomainWwwCard'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['domains'],
  defaultNS: 'domains',
  resources: { en: { domains: domainsEn } },
  interpolation: { escapeValue: false },
})

function plan(provider: string, outcome = 'created') {
  return {
    domain: 'a.com',
    provider,
    zone: 'a.com.',
    mode: 'both',
    canonical: 'apex',
    records: [
      {
        name: 'www',
        type: 'CNAME',
        value: 'a.com',
        ttl_seconds: 300,
        outcome,
        replaces: [],
      },
    ],
    redirect: { from: 'www.a.com', to: 'a.com' },
    message: '',
  }
}

function renderCard(domain = 'a.com') {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <I18nextProvider i18n={testI18n}>
        <DomainWwwCard appName="app" domain={domain} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('DomainWwwCard', () => {
  it('shows records and Apply for me with a provider', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(Response.json(plan('cloudflare')))),
    )
    renderCard()
    expect(await screen.findByText('CNAME')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Apply for me' })).toBeTruthy()
    expect(screen.getByText('Send www visitors to a.com')).toBeTruthy()
  })

  it('offers copy only when no provider is connected', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(Response.json(plan('')))),
    )
    renderCard()
    await screen.findByText(/No DNS provider is connected/)
    expect(screen.queryByRole('button', { name: 'Apply for me' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Check again' })).toBeTruthy()
  })

  it('warns and replaces on conflict', async () => {
    const fetchMock = vi.fn((_url: string, init?: RequestInit) =>
      Promise.resolve(
        init?.method === 'POST'
          ? Response.json({ applied: true, records: [], redirect: null })
          : Response.json(plan('cloudflare', 'conflict')),
      ),
    )
    vi.stubGlobal('fetch', fetchMock)
    renderCard()
    await userEvent.click(
      await screen.findByRole('button', { name: 'Replace and apply' }),
    )
    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some((c) =>
          /replace.{1,3}true/.test(JSON.stringify(c[1])),
        ),
      ).toBe(true),
    )
  })

  it('hides for www and automatic hostnames', () => {
    const fetchMock = vi.fn()
    vi.stubGlobal('fetch', fetchMock)
    const { container } = renderCard('1-2-3-4.sslip.io')
    expect(container.firstChild).toBeNull()
    expect(fetchMock).not.toHaveBeenCalled()
  })
})
