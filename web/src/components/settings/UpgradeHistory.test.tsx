import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import updatesEn from '../../locales/en/updates.json'
import type {
  UpgradeHistory as History,
  UpgradeHistoryItem,
} from '../../queries/upgradeHistory'
import { UpgradeHistory } from './UpgradeHistory'
import { createTestQueryClient } from '@/test/testQueryClient'

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
  const qc = createTestQueryClient()
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

  it('shows a compact row, then details with rollback and release link', async () => {
    renderWith({
      current_version: 'v0.0.2',
      unacknowledged: 1,
      entries: [base],
      agent_changes: [],
    })
    expect(
      await screen.findByRole('button', { name: 'Acknowledge v0.0.2' }),
    ).toBeInTheDocument()
    expect(screen.getByText('1 change to review')).toBeInTheDocument()
    expect(screen.getByText('Unknown initiator')).toBeInTheDocument()
    expect(screen.getByText('Schema 10 to 12')).toBeInTheDocument()
    expect(screen.getByText('No backup, schema changed')).toBeInTheDocument()
    expect(screen.queryByText(/Roll back to/)).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Details for v0.0.2' }))
    expect(screen.getByText(/database schema changed/i)).toBeInTheDocument()
    expect(
      screen.getByRole('button', { name: /Roll back to v0.0.1/ }),
    ).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Release page/ })).toHaveAttribute(
      'href',
      base.release_url,
    )
  })

  it('acknowledges a row and announces it', async () => {
    const acked = { ...base, acknowledged: true, acked_by: 'dev' }
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({
            current_version: 'v0.0.2',
            unacknowledged: 1,
            entries: [base],
            agent_changes: [],
          }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: () => Promise.resolve(acked),
      })
      .mockResolvedValue({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({
            current_version: 'v0.0.2',
            unacknowledged: 0,
            entries: [acked],
            agent_changes: [],
          }),
      })
    vi.stubGlobal('fetch', fetchMock)
    render(
      <I18nextProvider i18n={testI18n}>
        <QueryClientProvider client={createTestQueryClient()}>
          <UpgradeHistory />
        </QueryClientProvider>
      </I18nextProvider>,
    )
    fireEvent.click(
      await screen.findByRole('button', { name: 'Acknowledge v0.0.2' }),
    )
    expect(
      await screen.findByText('Upgrade to v0.0.2 acknowledged'),
    ).toBeInTheDocument()
    expect(await screen.findByText('Acknowledged')).toBeInTheDocument()
  })

  it('caps at 10 rows and reveals the rest with Show all', async () => {
    const entries = Array.from({ length: 12 }, (_, i) => ({
      ...base,
      id: `uh_${String(i)}`,
      to_version: `v1.0.${String(i)}`,
      acknowledged: true,
    }))
    renderWith({
      current_version: 'v1.0.0',
      unacknowledged: 0,
      entries,
      agent_changes: [],
    })
    expect(await screen.findAllByRole('listitem')).toHaveLength(10)
    fireEvent.click(screen.getByRole('button', { name: 'Show all 12' }))
    expect(screen.getAllByRole('listitem')).toHaveLength(12)
    fireEvent.click(screen.getByRole('button', { name: 'Show latest 10' }))
    expect(screen.getAllByRole('listitem')).toHaveLength(10)
  })
})
