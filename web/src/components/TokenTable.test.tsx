import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import { TokenTable } from './TokenTable'
import settingsEn from '../locales/en/settings.json'
import signInEn from '../locales/en/signIn.json'
import commonEn from '../locales/en/common.json'
import { BrandContext } from '../lib/brandContext'
import type { Brand } from '../types/brand'
import type { TokenResource } from '../types/token'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings', 'signIn', 'common'],
  defaultNS: 'settings',
  resources: {
    en: { settings: settingsEn, signIn: signInEn, common: commonEn },
  },
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

function renderTable(tokens: TokenResource[], rotatableIds?: Set<string>) {
  const client = new QueryClient()
  return render(
    <QueryClientProvider client={client}>
      <BrandContext.Provider value={{ BinaryName: 'testcli' } as Brand}>
        <I18nextProvider i18n={testI18n}>
          <TokenTable tokens={tokens} rotatableIds={rotatableIds} />
        </I18nextProvider>
      </BrandContext.Provider>
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

  it('groups dev fixtures in a collapsed section below real tokens', () => {
    renderTable([
      token({ id: 'real', name: 'ci-deploy' }),
      token({ id: 'fx', name: '[dev fixture] dev-read', dev_fixture: true }),
    ])
    expect(screen.getByText('ci-deploy')).toBeInTheDocument()
    expect(screen.queryByText('[dev fixture] dev-read')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: /Dev fixture tokens/ }))
    expect(screen.getByText(/Safe to ignore/)).toBeInTheDocument()
    expect(screen.getByText('[dev fixture] dev-read')).toBeInTheDocument()
    expect(screen.getByText('Dev fixture, local only')).toBeInTheDocument()
    expect(screen.queryByText(/publicly known secret/)).toBeNull()
  })

  it('warns harder on the root fixture', () => {
    renderTable([
      token({
        id: 'fx',
        name: '[dev fixture] dev-root',
        abilities: ['root'],
        dev_fixture: true,
      }),
    ])
    fireEvent.click(screen.getByRole('button', { name: /Dev fixture tokens/ }))
    expect(
      screen.getByText(/publicly known secret. Never expose this server/),
    ).toBeInTheDocument()
  })

  it('says a revoked fixture is reseeded on the next dev-mode start', () => {
    renderTable([
      token({ id: 'fx', name: '[dev fixture] dev-read', dev_fixture: true }),
    ])
    fireEvent.click(screen.getByRole('button', { name: /Dev fixture tokens/ }))
    fireEvent.click(screen.getByRole('button', { name: 'Revoke' }))
    expect(
      screen.getByText(/reseeded on the next dev-mode start/),
    ).toBeVisible()
  })

  it('offers Rotate only on CLI tokens the caller may rotate', () => {
    renderTable(
      [
        token({ id: 'a', name: 'cli-pairing laptop' }),
        token({ id: 'b', name: 'cli-pairing other' }),
        token({ id: 'c', name: 'ci-deploy' }),
      ],
      new Set(['a', 'c']),
    )
    expect(
      screen.getByRole('button', { name: 'Rotate cli-pairing laptop' }),
    ).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Rotate cli-pairing other' }),
    ).toBeNull()
    expect(
      screen.queryByRole('button', { name: 'Rotate ci-deploy' }),
    ).toBeNull()
  })

  it('opens the rotate dialog from the table', () => {
    renderTable(
      [token({ id: 'a', name: 'cli-pairing laptop' })],
      new Set(['a']),
    )
    fireEvent.click(
      screen.getByRole('button', { name: 'Rotate cli-pairing laptop' }),
    )
    expect(screen.getByText('Rotate this CLI token?')).toBeVisible()
  })
})
