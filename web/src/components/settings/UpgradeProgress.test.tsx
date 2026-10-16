import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import updatesEn from '../../locales/en/updates.json'
import type { SelfUpgradeAttempt } from '../../queries/selfUpgrade'
import { UpgradeProgress } from './UpgradeProgress'
import { progressRows } from './upgradeProgressRows'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['updates'],
  defaultNS: 'updates',
  resources: { en: { updates: updatesEn } },
  interpolation: { escapeValue: false },
})

const planned = ['download', 'verify_checksum', 'stop', 'health']

function attempt(over: Partial<SelfUpgradeAttempt>): SelfUpgradeAttempt {
  return {
    id: 'su-1',
    from_version: 'v0.3.0',
    to_version: 'v0.4.0',
    from_schema: 1,
    to_schema: 2,
    initiator: 'dashboard:a',
    outcome: 'running',
    failed_step: '',
    error: '',
    backup_name: '',
    acked: [],
    steps: [],
    current_step: '',
    log: [],
    started_at: '2026-10-15T10:00:00Z',
    finished_at: '',
    ...over,
  }
}

describe('progressRows', () => {
  it('marks done, running and pending steps in order', () => {
    const rows = progressRows(
      planned,
      attempt({
        current_step: 'verify_checksum',
        steps: [
          { name: 'guard', status: 'ok', detail: '', at: '', duration_ms: 1 },
          {
            name: 'download',
            status: 'ok',
            detail: 'x',
            at: '',
            duration_ms: 5,
          },
        ],
      }),
    )
    expect(rows.map((r) => r.state)).toEqual([
      'ok',
      'ok',
      'running',
      'pending',
      'pending',
    ])
  })
})

describe('UpgradeProgress', () => {
  const wrap = (a: SelfUpgradeAttempt | undefined, reconnecting = false) =>
    render(
      <I18nextProvider i18n={testI18n}>
        <UpgradeProgress
          target="v0.4.0"
          plannedSteps={planned}
          attempt={a}
          startedAt={Date.now()}
          reconnecting={reconnecting}
          releaseUrl="https://example.test/r"
        />
      </I18nextProvider>,
    )

  it('shows a reconnecting notice while the control plane restarts', () => {
    wrap(attempt({}), true)
    expect(screen.getByText(/reconnecting/)).toBeTruthy()
  })

  it('shows the failed step, error and rollback note', () => {
    wrap(
      attempt({
        outcome: 'rolled_back',
        failed_step: 'health',
        error: 'not healthy',
        steps: [
          {
            name: 'health',
            status: 'failed',
            detail: '',
            at: '',
            duration_ms: 1,
          },
        ],
        finished_at: '2026-10-15T10:01:00Z',
      }),
    )
    expect(screen.getByText(/Stopped at: Health check/)).toBeTruthy()
    expect(screen.getByText('not healthy')).toBeTruthy()
    expect(
      screen.getByText(/Rolled back to v0.3.0, your data is intact/),
    ).toBeTruthy()
  })

  it('links the release notes on success', () => {
    wrap(attempt({ outcome: 'succeeded', finished_at: '2026-10-15T10:01:00Z' }))
    expect(screen.getByRole('link').getAttribute('href')).toBe(
      'https://example.test/r',
    )
  })
})
