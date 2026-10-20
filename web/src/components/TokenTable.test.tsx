import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import { TokenTable } from './TokenTable'
import settingsEn from '../locales/en/settings.json'
import type { TokenResource } from '../types/token'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn } },
  interpolation: { escapeValue: false },
})

function token(overrides: Partial<TokenResource>): TokenResource {
  return {
    id: 'tok_1',
    name: 'cli login: laptop',
    abilities: ['read'],
    created_at: new Date(Date.now() - 3_600_000).toISOString(),
    ...overrides,
  }
}

function renderTable(tokens: TokenResource[]) {
  const client = new QueryClient()
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <TokenTable tokens={tokens} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

describe('TokenTable', () => {
  it('shows relative times and never-used tokens as Never', () => {
    renderTable([token({})])
    expect(screen.getByText('1h ago')).toBeInTheDocument()
    expect(screen.getAllByText('Never')).toHaveLength(2)
  })

  it('marks a token used in the last five minutes as active now', () => {
    renderTable([
      token({ last_used_at: new Date(Date.now() - 60_000).toISOString() }),
    ])
    expect(
      screen.getByText('Active now', { selector: 'div,span' }),
    ).toBeInTheDocument()
    expect(screen.getByText('1m ago')).toBeInTheDocument()
  })

  it('shows an idle token as plain Active and a future expiry as relative', () => {
    renderTable([
      token({
        last_used_at: new Date(Date.now() - 2 * 3_600_000).toISOString(),
        expires_at: new Date(Date.now() + 30 * 86_400_000).toISOString(),
      }),
    ])
    expect(screen.getByText('Active')).toBeInTheDocument()
    expect(screen.getByText('in 30d')).toBeInTheDocument()
  })

  it('renders the empty state copy from the locale', () => {
    renderTable([])
    expect(screen.getByText('No tokens yet')).toBeInTheDocument()
  })
})
