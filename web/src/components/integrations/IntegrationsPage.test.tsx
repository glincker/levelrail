import type { ReactNode } from 'react'
import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import integrationsEn from '../../locales/en/integrations.json'
import type { CatalogEntry, Connection } from '../../types/connections'
import { ConnectionCard } from './ConnectionCard'
import { IntegrationsPage } from './IntegrationsPage'

const state = vi.hoisted(() => ({
  conns: {
    data: null as Connection[] | null | undefined,
    isPending: false,
    isError: false,
  },
  catalog: {
    data: null as CatalogEntry[] | null | undefined,
    isPending: false,
    isError: false,
  },
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    to,
    ...rest
  }: {
    children?: ReactNode
    to: string
  } & Record<string, unknown>) => (
    <a href={to} {...rest}>
      {children}
    </a>
  ),
}))
vi.mock('@/components/shell/PageTemplate', () => ({
  PageTemplate: ({ children }: { children: ReactNode }) => (
    <div>{children}</div>
  ),
}))
vi.mock('../../queries/connections', () => ({
  useConnections: () => ({ ...state.conns, refetch: vi.fn() }),
  useConnectionCatalog: () => ({ ...state.catalog, refetch: vi.fn() }),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'integrations'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, integrations: integrationsEn } },
  interpolation: { escapeValue: false },
})

const NOW = Date.parse('2026-11-15T12:00:00Z')
const base: Connection = {
  ref: 'git:1',
  kind: 'git',
  provider: 'github',
  label: 'acme-org',
  auth_method: 'token',
  capabilities: null,
  bound_count: 2,
  scope_type: 'org',
  scope_id: 'acme',
  health: 'ok',
  expiring_soon: false,
}

function wrap(node: ReactNode) {
  return render(<I18nextProvider i18n={i18n}>{node}</I18nextProvider>)
}

describe('ConnectionCard', () => {
  it('shows days until expiry, scope and bindings', () => {
    wrap(
      <ul>
        <ConnectionCard
          conn={{
            ...base,
            expires_at: new Date(NOW + 5 * 86_400_000).toISOString(),
            expiring_soon: true,
          }}
          now={NOW}
        />
      </ul>,
    )
    expect(screen.getByText('Expires in 5 days')).toBeTruthy()
    expect(screen.getByText('Org: acme')).toBeTruthy()
    expect(screen.getByText('2 bindings')).toBeTruthy()
  })
  it('offers reconnect and the auto renew note for oauth', () => {
    wrap(
      <ul>
        <ConnectionCard
          conn={{
            ...base,
            auth_method: 'oauth',
            health: 'needs_reconnect',
            expires_at: new Date(NOW + 86_400_000).toISOString(),
          }}
          now={NOW}
        />
      </ul>,
    )
    expect(
      screen.getByRole('link', { name: 'Reconnect acme-org' }),
    ).toBeTruthy()
    expect(screen.getByText(/auto renews/)).toBeTruthy()
  })
})

describe('IntegrationsPage states', () => {
  it('treats null lists as empty without crashing', () => {
    state.conns = { data: null, isPending: false, isError: false }
    state.catalog = { data: null, isPending: false, isError: false }
    wrap(<IntegrationsPage />)
    expect(screen.getByText('Nothing connected yet')).toBeTruthy()
    expect(screen.getByText('No integrations are available.')).toBeTruthy()
  })
  it('treats empty arrays the same', () => {
    state.conns = { data: [], isPending: false, isError: false }
    state.catalog = { data: [], isPending: false, isError: false }
    wrap(<IntegrationsPage />)
    expect(screen.getByText('Nothing connected yet')).toBeTruthy()
  })
  it('shows an honest error with retry', () => {
    state.conns = { data: undefined, isPending: false, isError: true }
    state.catalog = { data: undefined, isPending: false, isError: true }
    wrap(<IntegrationsPage />)
    expect(screen.getByText('Could not load connections.')).toBeTruthy()
    expect(screen.getByText('Could not load the catalogue.')).toBeTruthy()
    expect(screen.getAllByRole('button', { name: 'Retry' })).toHaveLength(2)
  })
  it('lists connections and the attention strip', () => {
    state.conns = {
      data: [{ ...base, health: 'expired' }],
      isPending: false,
      isError: false,
    }
    state.catalog = {
      data: [
        {
          id: 'github',
          category: 'git',
          name: 'GitHub',
          connect_methods: ['one_click'],
        },
      ],
      isPending: false,
      isError: false,
    }
    wrap(<IntegrationsPage />)
    expect(screen.getByText('1 connection needs attention')).toBeTruthy()
    expect(screen.getByRole('link', { name: 'Connect GitHub' })).toBeTruthy()
  })
})
