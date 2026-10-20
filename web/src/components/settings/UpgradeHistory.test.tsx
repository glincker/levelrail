import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import updatesEn from '../../locales/en/updates.json'
import type {
  UpgradeHistory as History,
  UpgradeHistoryItem,
} from '../../queries/upgradeHistory'
import { UpgradeHistory } from './UpgradeHistory'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['updates'],
  defaultNS: 'updates',
  resources: { en: { updates: updatesEn } },
  interpolation: { escapeValue: false },
})

const base: UpgradeHistoryItem = {
  id: 'uh_1',
  kind: 'upgraded',
  from_version: 'v0.0.1',
  to_version: 'v0.0.2',
  channel: 'stable',
  schema_before: 10,
  schema_after: 12,
  schema_moved: true,
  occurred_at: '2026-10-09T00:00:00Z',
  initiator: 'unknown',
  method: '',
  backup_name: '',
  health: 'booted',
  notes: '',
  notes_state: 'unavailable',
  release_url: 'https://example.test/releases/tag/v0.0.2',
  compare_url: '',
  acknowledged: false,
  acked_by: '',
  acked_at: null,
  rollback_available: true,
}

function renderWith(data: History) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(data),
      }),
    ),
  )
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={qc}>
        <UpgradeHistory />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('UpgradeHistory', () => {
  it('shows a real empty state', async () => {
    renderWith({
      current_version: 'v0.0.1',
      unacknowledged: 0,
      entries: [],
      agent_changes: [],
    })
    expect(
      await screen.findByText(/No upgrades recorded yet/),
    ).toBeInTheDocument()
  })

  it('flags an unacknowledged upgrade, an unknown initiator and a moved schema', async () => {
    renderWith({
      current_version: 'v0.0.2',
      unacknowledged: 1,
      entries: [base],
      agent_changes: [],
    })
    expect(
      await screen.findByRole('button', { name: /Acknowledge/ }),
    ).toBeInTheDocument()
    expect(screen.getByText(/Initiator unknown/)).toBeInTheDocument()
    expect(screen.getByText(/database schema changed/i)).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Roll back to v0.0.1/ }),
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Release page/ })).toHaveAttribute(
      'href',
      base.release_url,
    )
  })
})
