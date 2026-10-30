import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import updatesEn from '../../locales/en/updates.json'
import type { UpgradePolicy } from '../../queries/upgradeSchedule'
import { AutomaticUpgrades } from './AutomaticUpgrades'
import { UpgradeAttempts } from './UpgradeAttempts'
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

const off: UpgradePolicy = {
  mode: 'off',
  effective: 'off',
  window_cron: '',
  window_duration_minutes: 120,
  timezone: 'UTC',
  min_release_age_days: 3,
  notify_channel_ids: [],
  paused: false,
  server_timezone: 'UTC',
  next_window: null,
}

function mount(
  ui: React.ReactElement,
  handler: (url: string, init?: RequestInit) => unknown,
) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(handler(url, init)),
      }),
    ),
  )
  const qc = createTestQueryClient()
  render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={qc}>{ui}</QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('AutomaticUpgrades', () => {
  it('states truthfully that nothing upgrades itself while off', async () => {
    mount(<AutomaticUpgrades />, (url) =>
      url.includes('notification-channels') ? [] : off,
    )
    expect(
      await screen.findByText(
        /Automatic upgrades are off\. This control plane upgrades only when you start or schedule/,
      ),
    ).toBeInTheDocument()
    expect(
      screen.queryByLabelText('Cron (start of window)'),
    ).not.toBeInTheDocument()
  })

  it('saves a patch-only policy with a window', async () => {
    const puts: string[] = []
    mount(<AutomaticUpgrades />, (url, init) => {
      if (url.includes('notification-channels')) {
        return [{ id: 'ch_1', name: 'Ops Slack', kind: 'slack', enabled: true }]
      }
      if (init?.method === 'PUT') {
        puts.push(typeof init.body === 'string' ? init.body : '')
        return {
          ...off,
          mode: 'patch',
          effective: 'patch',
          window_cron: '0 3 * * *',
        }
      }
      return off
    })
    await screen.findByText(/Automatic upgrades are off/)
    await userEvent.click(
      screen.getByRole('combobox', { name: 'Automatic upgrades' }),
    )
    await userEvent.click(
      await screen.findByRole('option', { name: /Patch releases only/ }),
    )
    await userEvent.click(
      await screen.findByRole('checkbox', { name: 'Ops Slack' }),
    )
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(puts).toHaveLength(1)
    })
    expect(JSON.parse(puts[0] ?? '{}')).toMatchObject({
      mode: 'patch',
      window_cron: '0 3 * * *',
      window_duration_minutes: 120,
      min_release_age_days: 3,
      notify_channel_ids: ['ch_1'],
      paused: false,
    })
  })

  it('shows why a release is being held', async () => {
    mount(<AutomaticUpgrades />, (url) =>
      url.includes('notification-channels')
        ? []
        : {
            ...off,
            mode: 'any',
            effective: 'any',
            window_cron: '0 3 * * *',
            decision_tag: 'v1.2.0',
            decision:
              'held: v1.2.0 has 1 breaking change(s) that need your acknowledgement',
          },
    )
    expect(
      await screen.findByText(/Latest decision for v1.2.0: held/),
    ).toBeInTheDocument()
  })
})

describe('UpgradeAttempts', () => {
  it('lists a rolled back attempt with its failed step', async () => {
    mount(<UpgradeAttempts />, () => ({
      attempts: [
        {
          id: 'su-1',
          from_version: 'v0.9.0',
          to_version: 'v1.0.0',
          from_schema: 10,
          to_schema: 11,
          initiator: 'schedule:us_1',
          outcome: 'rolled_back',
          failed_step: 'health',
          error: 'new release did not become healthy',
          backup_name: 'b.db',
          acked: [],
          steps: [
            {
              name: 'health',
              status: 'failed',
              detail: 'timeout',
              at: '',
              duration_ms: 1,
            },
          ],
          started_at: '2026-10-10T00:00:00Z',
          finished_at: '2026-10-10T00:01:00Z',
        },
      ],
    }))
    expect(await screen.findByText('Rolled back')).toBeInTheDocument()
    expect(screen.getByText('timeout')).toBeInTheDocument()
    expect(
      screen.getByText('new release did not become healthy'),
    ).toBeInTheDocument()
  })
})

describe('AutomaticUpgrades on the edge channel', () => {
  it('explains that edge is never upgraded automatically', async () => {
    mount(<AutomaticUpgrades />, (url) =>
      url.endsWith('/updates/settings')
        ? { channel: 'edge', auto_update_enabled: true }
        : off,
    )
    expect(
      await screen.findByText(/not available on the edge channel/),
    ).toBeInTheDocument()
    expect(
      screen.queryByLabelText('Automatic upgrades'),
    ).not.toBeInTheDocument()
  })
})
