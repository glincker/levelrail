import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { PasswordRotationCard } from './PasswordRotationCard'
import accessEn from '../../locales/en/databaseAccess.json'
import type { DatabasePasswordRotation } from '../../queries/databasePasswordRotation'
import { createTestQueryClient } from '@/test/testQueryClient'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['databaseAccess', 'common'],
  defaultNS: 'databaseAccess',
  resources: { en: { databaseAccess: accessEn, common: {} } },
  interpolation: { escapeValue: false },
})

function respondWith(body: DatabasePasswordRotation) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(new Response(JSON.stringify(body)))),
  )
}

function renderCard() {
  const client = createTestQueryClient()
  return render(
    <QueryClientProvider client={client}>
      <I18nextProvider i18n={testI18n}>
        <PasswordRotationCard databaseName="main" />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

const base: DatabasePasswordRotation = {
  database: 'main',
  engine: 'mysql',
  supported: true,
  restart_required: true,
  apps: ['web'],
  suspended_apps: [],
  state: 'idle',
  interval_days: 0,
}

describe('PasswordRotationCard', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('offers rotation and shows the password age for a supported engine', async () => {
    respondWith({
      ...base,
      last_changed_at: '2026-10-01T00:00:00Z',
      age_days: 18,
    })
    renderCard()
    expect(
      await screen.findByText('Password changed 18 days ago.'),
    ).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Rotate password' })).toBeTruthy()
  })

  it('explains an unsupported engine instead of offering a button', async () => {
    respondWith({
      ...base,
      engine: 'redis',
      supported: false,
      reason: 'no password',
    })
    renderCard()
    expect(
      await screen.findByText('Rotation is not available for this engine'),
    ).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Rotate password' })).toBeNull()
  })

  it('tells the operator when the last attempt was rolled back', async () => {
    respondWith({ ...base, state: 'failed', last_error: 'verify_failed' })
    renderCard()
    expect(
      await screen.findByText(/the database refused the new password/),
    ).toBeTruthy()
  })
})
