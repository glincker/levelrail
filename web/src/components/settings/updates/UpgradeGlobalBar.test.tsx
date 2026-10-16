import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import updatesEn from '../../../locales/en/updates.json'
import type { UpgradeStatus } from '../../../hooks/useUpgradeStatus'
import { buildPipeline } from '../../../lib/upgradePhases'
import { UpgradeGlobalBar } from './UpgradeGlobalBar'
import { UpgradePipeline } from './UpgradePipeline'

const mockStatus = vi.hoisted((): { value: UpgradeStatus | null } => ({
  value: null,
}))
vi.mock('../../../hooks/useUpgradeStatus', () => ({
  useUpgradeStatus: () => mockStatus.value,
}))
vi.mock('@tanstack/react-router', () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}))

const i18n = i18next.createInstance()
void i18n.use(initReactI18next).init({
  lng: 'en',
  ns: ['updates'],
  defaultNS: 'updates',
  resources: { en: { updates: updatesEn } },
  interpolation: { escapeValue: false },
})

function status(over: Partial<UpgradeStatus>): UpgradeStatus {
  return {
    state: 'running',
    target: 'v0.5.1',
    attempt: undefined,
    view: buildPipeline([], undefined),
    startedAt: Date.now(),
    lastStep: 'download',
    ...over,
  }
}

function mockMatchMedia(reduce: boolean) {
  window.matchMedia = ((q: string) => ({
    matches: reduce && q.includes('reduce'),
    media: q,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  })) as unknown as typeof window.matchMedia
}

const bar = () =>
  render(
    <I18nextProvider i18n={i18n}>
      <UpgradeGlobalBar />
    </I18nextProvider>,
  )

afterEach(() => mockMatchMedia(false))

describe('UpgradeGlobalBar', () => {
  it('renders nothing when idle', () => {
    mockStatus.value = status({ state: 'idle' })
    const { container } = bar()
    expect(container).toBeEmptyDOMElement()
  })

  it('shows progress and a link while running', () => {
    mockStatus.value = status({})
    bar()
    expect(screen.getByRole('progressbar')).toBeInTheDocument()
    expect(screen.getByText('Download')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'View progress' })).toHaveAttribute(
      'href',
      '/settings/updates',
    )
  })

  it('shows reconnecting instead of an error after a known running state', () => {
    mockStatus.value = status({ state: 'reconnecting' })
    bar()
    expect(
      screen.getByText(/Restarting the control plane, reconnecting/),
    ).toBeInTheDocument()
  })

  it('uses no animation classes under reduced motion', () => {
    mockMatchMedia(true)
    mockStatus.value = status({ state: 'reconnecting' })
    const { container } = bar()
    expect(container.innerHTML).not.toMatch(/upg-|animate-spin/)
  })

  it('animates when motion is allowed', () => {
    mockStatus.value = status({ state: 'reconnecting' })
    const { container } = bar()
    expect(container.innerHTML).toMatch(/upg-breathe/)
  })
})

describe('UpgradePipeline', () => {
  it('shows the rollback banner', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <UpgradePipeline
          target="v0.5.1"
          plannedSteps={[]}
          startedAt={null}
          reconnecting={false}
          attempt={{
            id: 'a',
            from_version: 'v0.5.0',
            to_version: 'v0.5.1',
            from_schema: 1,
            to_schema: 1,
            initiator: '',
            outcome: 'rolled_back',
            failed_step: 'health',
            error: 'x',
            backup_name: '',
            acked: [],
            steps: [],
            current_step: '',
            log: [],
            started_at: '2026-10-15T10:00:00Z',
            finished_at: '2026-10-15T10:01:00Z',
          }}
        />
      </I18nextProvider>,
    )
    expect(
      screen.getByText('Rolled back to v0.5.0, your data is intact.'),
    ).toBeInTheDocument()
  })
})
