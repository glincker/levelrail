import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import settingsEn from '../../locales/en/settings.json'
import commonEn from '../../locales/en/common.json'
import type { IntegrityView } from '../../queries/dbIntegrity'
import { IntegrityCard } from './IntegrityCard'
import { createTestQueryClient } from '@/test/testQueryClient'

vi.mock('@/hooks/useIsRoot', () => ({ useIsRoot: () => true }))

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['settings', 'common'],
  defaultNS: 'settings',
  resources: { en: { settings: settingsEn, common: commonEn } },
  interpolation: { escapeValue: false },
})

function renderWith(data: IntegrityView) {
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
        <IntegrityCard />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('IntegrityCard', () => {
  it('shows damage, the error, and a restore button per snapshot', async () => {
    renderWith({
      status: {
        healthy: false,
        mode: 'quick',
        checked_at: '2026-10-19T10:00:00Z',
        error: 'database file is corrupt: page 3',
        checks: 2,
        failures: 1,
        last_full_at: '0001-01-01T00:00:00Z',
      },
      candidates: [
        {
          name: 'levelrail-20261019.db',
          size_bytes: 4096,
          created_at: '2026-10-19T03:00:00Z',
          verified: true,
        },
      ],
    })
    expect(await screen.findByText('Damaged')).toBeTruthy()
    expect(screen.getByText('database file is corrupt: page 3')).toBeTruthy()
    expect(screen.getByText('levelrail-20261019.db')).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Restore' })).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Full check now' })).toBeTruthy()
  })

  it('announces a staged restore with a way to cancel it', async () => {
    renderWith({
      status: {
        healthy: true,
        mode: 'quick',
        checked_at: '2026-10-19T10:00:00Z',
        checks: 1,
        failures: 0,
        last_full_at: '0001-01-01T00:00:00Z',
      },
      pending: {
        name: 'snap.db',
        sha256: 'x',
        schema_version: 7,
        staged_at: '2026-10-19T10:01:00Z',
      },
      candidates: [],
    })
    expect(
      await screen.findByText(
        'Snapshot snap.db replaces the database at the next start.',
      ),
    ).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeTruthy()
  })
})
