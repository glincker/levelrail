import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DatabasesPage } from './DatabasesPage'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import commonEn from '../locales/en/common.json'
import databasesEn from '../locales/en/databases.json'

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'databases'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, databases: databasesEn } },
  interpolation: { escapeValue: false },
})

function Providers({ children }: Readonly<{ children: ReactNode }>) {
  const client = new QueryClient()
  return (
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={i18n}>{children}</I18nextProvider>
    </QueryClientProvider>
  )
}

vi.mock('./DatabasesTable', () => ({
  DatabasesTable: () => <div data-testid="databases-table" />,
}))

// The real dialog reads route search params, which needs a router.
vi.mock('./CreateDatabaseDialog', () => ({
  CreateDatabaseDialog: () => <button type="button">New database</button>,
}))

describe('DatabasesPage', () => {
  it('shows the hero banner with one create action and no empty-state card', () => {
    render(
      <Providers>
        <DatabasesPage databases={[]} />
      </Providers>,
    )
    expect(screen.getByTestId('page-banner-full')).toBeTruthy()
    expect(
      screen.getAllByRole('button', { name: /new database/i }),
    ).toHaveLength(1)
    expect(screen.queryByText('No databases yet')).toBeNull()
    expect(screen.queryByTestId('databases-table')).toBeNull()
  })

  it('shows a compact banner above the table when databases exist', () => {
    render(
      <Providers>
        <DatabasesPage
          databases={[
            {
              name: 'main',
              engine: 'postgres',
              version: '16',
              status: { label: 'Running', variant: 'success' },
            },
          ]}
        />
      </Providers>,
    )
    expect(screen.getByTestId('page-banner-compact')).toBeTruthy()
    expect(screen.getByTestId('databases-table')).toBeTruthy()
    expect(
      screen.getAllByRole('button', { name: /new database/i }),
    ).toHaveLength(1)
  })
})
