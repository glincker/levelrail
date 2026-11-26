import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it, vi } from 'vitest'
import databaseUpgrades from '../../locales/en/databaseUpgrades.json'
import type { DatabaseUpgrades } from '../../types/databaseUpgrades'
import { CurrentVersionCard } from './CurrentVersionCard'

vi.mock('../../queries/databases', () => ({
  useDatabase: () => ({ data: { name: 'main', engine: 'postgres' } }),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  resources: { en: { databaseUpgrades } },
  defaultNS: 'databaseUpgrades',
})

function data(blockers: string[]): DatabaseUpgrades {
  return {
    advice: {
      engine: 'postgres',
      current: '16.1',
      support: 'supported',
      catalog_updated: '2026-01-01',
    },
    blockers,
  } as unknown as DatabaseUpgrades
}

describe('CurrentVersionCard backup call to action', () => {
  it('turns the missing backup target blocker into a setup button', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <CurrentVersionCard
          databaseName="main"
          data={data([
            'no backup target is configured: every upgrade starts from a fresh backup, so none can run',
          ])}
        />
      </I18nextProvider>,
    )
    expect(
      screen.getByRole('button', { name: 'Set up backup storage' }),
    ).toBeInTheDocument()
    expect(screen.queryByText('Upgrades are blocked')).toBeNull()
  })
})
