import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import updatesEn from '../../../locales/en/updates.json'
import type {
  SelfUpgradeAttempt,
  SelfUpgradeStatus,
} from '../../../queries/selfUpgrade'
import { isSeen, markSeen } from '../../../lib/upgradeSeen'
import { UpgradeOutcomeToast } from './UpgradeOutcomeToast'
import { UpgradeResultCard } from './UpgradeResultCard'

const toastAdd = vi.hoisted(() => vi.fn())
vi.mock('../../ui/toast', () => ({ toast: { add: toastAdd } }))
vi.mock('../../../hooks/useIsRoot', () => ({ useIsRoot: () => true }))
vi.mock('./FailureActions', () => ({
  FailureActions: () => <button type="button">Retry</button>,
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['updates'],
  defaultNS: 'updates',
  resources: { en: { updates: updatesEn } },
  interpolation: { escapeValue: false },
})

function attempt(over: Partial<SelfUpgradeAttempt>): SelfUpgradeAttempt {
  return {
    id: 'a1',
    from_version: 'v0.7.0',
    to_version: 'v0.8.0',
    from_schema: 1,
    to_schema: 2,
    initiator: 'dashboard:me',
    outcome: 'succeeded',
    failed_step: '',
    error: '',
    backup_name: '',
    acked: [],
    steps: [],
    current_step: '',
    log: [],
    started_at: new Date().toISOString(),
    finished_at: new Date().toISOString(),
    ...over,
  }
}

function renderToast(status: SelfUpgradeStatus) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(new Response(JSON.stringify(status)))),
  )
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <I18nextProvider i18n={i18n}>
        <UpgradeOutcomeToast />
      </I18nextProvider>
    </QueryClientProvider>,
  )
}

beforeEach(() => {
  localStorage.clear()
  toastAdd.mockClear()
})
afterEach(() => vi.unstubAllGlobals())

describe('UpgradeResultCard', () => {
  it('shows a compact success card without a Reload button', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <UpgradeResultCard
          attempt={attempt({})}
          verified
          releaseUrl="https://example.test/notes"
          onDismiss={() => undefined}
        />
      </I18nextProvider>,
    )
    expect(screen.getByText(/Upgraded from v0.7.0 to v0.8.0 at/)).toBeTruthy()
    expect(screen.getByText(/verification passed/)).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Reload' })).toBeNull()
    expect(screen.getByText('View attempt details')).toBeTruthy()
  })

  it('expands the timeline on demand and still hides Reload', async () => {
    render(
      <I18nextProvider i18n={i18n}>
        <UpgradeResultCard
          attempt={attempt({})}
          verified
          releaseUrl={null}
          onDismiss={() => undefined}
        />
      </I18nextProvider>,
    )
    await userEvent.click(screen.getByText('View attempt details'))
    await waitFor(() =>
      expect(screen.getByText('Hide attempt details')).toBeTruthy(),
    )
    expect(screen.queryByRole('button', { name: 'Reload' })).toBeNull()
  })

  it('shows the reason and fix actions for a failed attempt', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <UpgradeResultCard
          attempt={attempt({
            outcome: 'failed',
            error: 'health check timed out',
          })}
          verified={false}
          releaseUrl={null}
          onDismiss={() => undefined}
        />
      </I18nextProvider>,
    )
    expect(screen.getByText(/health check timed out/)).toBeTruthy()
    expect(screen.getByRole('button', { name: 'Retry' })).toBeTruthy()
  })
})

describe('UpgradeOutcomeToast', () => {
  const status = (a: SelfUpgradeAttempt): SelfUpgradeStatus => ({
    current_version: 'v0.8.0',
    running: null,
    last_finished: a,
    verified: true,
  })

  it('toasts a succeeded-unseen outcome once and marks it seen', async () => {
    renderToast(status(attempt({})))
    await waitFor(() => expect(toastAdd).toHaveBeenCalledTimes(1))
    expect(isSeen('toast', 'a1')).toBe(true)
  })

  it('stays quiet for a succeeded-seen outcome after reload', async () => {
    markSeen('toast', 'a1')
    renderToast(status(attempt({})))
    await new Promise((r) => setTimeout(r, 50))
    expect(toastAdd).not.toHaveBeenCalled()
  })
})
