import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import updatesEn from '../../locales/en/updates.json'
import type {
  SelfUpgradeAttempt,
  SelfUpgradePlan,
} from '../../queries/selfUpgrade'
import { SelfUpgrade } from './SelfUpgrade'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['updates'],
  defaultNS: 'updates',
  resources: { en: { updates: updatesEn } },
  interpolation: { escapeValue: false },
})

const plan: SelfUpgradePlan = {
  current_version: 'v1.0.0',
  target_version: 'v1.1.0',
  breaking: [
    {
      id: 'ports',
      version: 'v1.1.0',
      summary: 'Ingress ports move',
      requires_ack: true,
    },
  ],
  notes_available: true,
  can_apply: true,
  command: 'sudo levelrail self-upgrade --to v1.1.0',
  steps: ['download', 'health'],
}

const rolledBack: SelfUpgradeAttempt = {
  id: 'su-1',
  from_version: 'v0.9.0',
  to_version: 'v1.0.0',
  from_schema: 10,
  to_schema: 11,
  initiator: 'dashboard:admin',
  outcome: 'rolled_back',
  failed_step: 'health',
  error: 'new release did not become healthy',
  backup_name: 'b.db',
  acked: [],
  steps: [
    {
      name: 'download',
      status: 'ok',
      detail: 'v1.0.0',
      at: '',
      duration_ms: 1,
    },
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
}

function stubFetch(
  p: SelfUpgradePlan,
  attempts: SelfUpgradeAttempt[],
  posts: string[],
) {
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      if (init?.method === 'POST') {
        posts.push(typeof init.body === 'string' ? init.body : '')
        return Promise.resolve({
          ok: true,
          status: 202,
          json: () =>
            Promise.resolve({ status: 'started', target_version: 'v1.1.0' }),
        })
      }
      const body = url.includes('/attempts') ? { attempts } : p
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(body),
      })
    }),
  )
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <I18nextProvider i18n={testI18n}>
      <QueryClientProvider client={qc}>
        <SelfUpgrade />
      </QueryClientProvider>
    </I18nextProvider>,
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('SelfUpgrade', () => {
  it('keeps the upgrade disabled until each breaking change is acknowledged', async () => {
    const posts: string[] = []
    stubFetch(plan, [], posts)
    const apply = await screen.findByRole('button', {
      name: 'Upgrade to v1.1.0',
    })
    expect(apply).toBeDisabled()
    expect(screen.getByText('Ingress ports move')).toBeInTheDocument()

    await userEvent.click(
      screen.getByRole('checkbox', { name: /Ingress ports move/ }),
    )
    await waitFor(() => {
      expect(apply).toBeEnabled()
    })
    await userEvent.click(apply)
    await waitFor(() => {
      expect(posts).toHaveLength(1)
    })
    expect(JSON.parse(posts[0] ?? '')).toEqual({
      target: 'v1.1.0',
      ack: ['ports'],
    })
  })

  it('shows the host command when the dashboard cannot apply', async () => {
    stubFetch(
      {
        ...plan,
        breaking: [],
        can_apply: false,
        cannot_apply_reason: 'not root',
      },
      [],
      [],
    )
    expect(
      await screen.findByText('sudo levelrail self-upgrade --to v1.1.0'),
    ).toBeInTheDocument()
    expect(screen.getByText(/not root/)).toBeInTheDocument()
    expect(
      screen.queryByRole('button', { name: 'Upgrade to v1.1.0' }),
    ).not.toBeInTheDocument()
  })

  it('lists a rolled back attempt with its failed step', async () => {
    stubFetch({ ...plan, breaking: [] }, [rolledBack], [])
    expect(await screen.findByText('Rolled back')).toBeInTheDocument()
    expect(screen.getByText('timeout')).toBeInTheDocument()
    expect(
      screen.getByText('new release did not become healthy'),
    ).toBeInTheDocument()
  })
})
