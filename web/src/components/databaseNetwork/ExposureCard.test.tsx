import { fireEvent, render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import dbNav from '../../locales/en/dbNav.json'
import type { DatabaseNetwork } from '../../types/databaseAccess'
import { ExposureCard } from './ExposureCard'
import { needsTypedConfirm } from './exposureLevel'

const mutations = vi.hoisted(() => ({
  makePrivate: vi.fn(),
  setPublic: vi.fn(),
}))

vi.mock('../../queries/databaseAccess', () => ({
  databaseAccessKeys: { all: (n: string) => ['db', n, 'access'] },
  useMakePrivate: () => ({ mutate: mutations.makePrivate, isPending: false }),
}))
vi.mock('../../queries/databases', () => ({
  useSetDatabasePublicAccess: () => ({
    mutate: mutations.setPublic,
    isPending: false,
  }),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { dbNav, common: {} } },
  defaultNS: 'dbNav',
  interpolation: { escapeValue: false },
})

function network(
  over: Partial<DatabaseNetwork['rules']> = {},
): DatabaseNetwork {
  return {
    database: 'orders',
    running: true,
    internal: { host: 'orders', port: 5432 },
    networks: [],
    clients: [],
    verdict: { level: 'private', text: '', clients: 0 },
    rules: { active: false, allow: [], can_restrict: true, ...over },
    scope: { current: 'platform' },
  } as unknown as DatabaseNetwork
}

function renderCard(n: DatabaseNetwork) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <I18nextProvider i18n={i18n}>
        <ExposureCard databaseName="orders" network={n} />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  mutations.makePrivate.mockReset()
  mutations.setPublic.mockReset()
})

describe('ExposureCard', () => {
  it('only enables Publish after typing the name', () => {
    renderCard(network())
    fireEvent.click(screen.getByRole('radio', { name: 'Public' }))
    fireEvent.click(screen.getByRole('button', { name: 'Publish' }))
    const confirm = screen
      .getAllByRole('button', { name: 'Publish' })
      .find((b) => b.closest('[role="dialog"]'))
    expect(confirm).toBeDisabled()
    fireEvent.change(screen.getByRole('textbox'), {
      target: { value: 'orders' },
    })
    expect(confirm).toBeEnabled()
    fireEvent.click(confirm as HTMLElement)
    expect(mutations.setPublic).toHaveBeenCalledWith(
      { name: 'orders', bindAddress: 'private' },
      expect.anything(),
    )
  })

  it('refuses Public with the host reason when it cannot restrict', () => {
    renderCard(
      network({
        can_restrict: false,
        cannot_restrict_reason: 'Docker Desktop',
      }),
    )
    fireEvent.click(screen.getByRole('radio', { name: 'Public' }))
    expect(screen.getByText(/Docker Desktop/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Publish' })).toBeDisabled()
  })

  it('requires the typed name for all interfaces even when already public', () => {
    expect(needsTypedConfirm('public', 'public')).toBe(true)
    expect(needsTypedConfirm('public', 'private')).toBe(false)
    expect(needsTypedConfirm('private', 'private')).toBe(true)
  })
})
