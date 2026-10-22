import { fireEvent, render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import commonEn from '../../locales/en/common.json'
import settingsEn from '../../locales/en/settings.json'
import type { CPHealth } from '../../queries/controlPlaneHealth'
import { ControlPlaneHealthPage } from './ControlPlaneHealthPage'

void i18next.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['common', 'settings'],
  defaultNS: 'common',
  resources: { en: { common: commonEn, settings: settingsEn } },
  interpolation: { escapeValue: false },
})

const state = vi.hoisted(() => ({ health: null as CPHealth | null }))
const mutate = vi.hoisted(() => vi.fn())

vi.mock('../../queries/controlPlaneHealth', () => ({
  useControlPlaneHealth: () => ({
    data: state.health,
    isPending: false,
    error: null,
    refetch: vi.fn(),
    isFetching: false,
  }),
  useApplyControlPlaneFix: () => ({ mutate, isPending: false }),
}))
vi.mock('@/components/settings/IntegrityCard', () => ({
  IntegrityCard: () => null,
}))
vi.mock('@/components/settings/ReconcileGuardCard', () => ({
  ReconcileGuardCard: () => null,
}))

function health(findings: CPHealth['findings']): CPHealth {
  return {
    checked_at: '2026-10-21T00:00:00Z',
    status: findings.length ? 'warn' : 'ok',
    runtime: { goroutines: 10, heap_bytes: 1 << 20, gc_pause_ms: 0, cpus: 2 },
    db: {
      queries: 1,
      slow_queries: 0,
      slow_threshold_ms: 200,
      p50_ms: 1,
      p95_ms: 2,
      p99_ms: 3,
      in_use: 0,
      wait_count: 0,
      wait_ms: 0,
      wal_bytes: 1 << 20,
      db_bytes: 1 << 20,
      recent_slow: [],
    },
    reconcile: {
      passes: 1,
      controllers: 1,
      last_pass_ms: 1,
      p50_ms: 1,
      p95_ms: 1,
      slowest_controller: 'app',
      lag_seconds: 0,
      pending: false,
      guard: {} as CPHealth['reconcile']['guard'],
    },
    streams: { active: 0, cap: 10 },
    footprint: {
      rss_bytes: 1 << 20,
      threads: 4,
      cpu_percent: 1,
      telemetry_bytes: 1 << 20,
      memory_limit: { limit_bytes: 0, source: '', percent: 0 },
      loops: [],
    },
    findings,
  }
}

describe('ControlPlaneHealthPage', () => {
  beforeEach(() => {
    mutate.mockClear()
  })

  it('uses the compact banner when everything is under its threshold', () => {
    state.health = health([])
    render(<ControlPlaneHealthPage />)
    expect(screen.getByTestId('page-banner-compact')).toBeTruthy()
    expect(
      screen.getByText(/Every measurement is under its threshold/),
    ).toBeTruthy()
  })

  it('loads the footprint card lazily when the server reports one', async () => {
    state.health = {
      ...health([]),
      idle_footprint: {
        status: 'over_tolerance',
        platform: 'linux/arm64',
        current: {
          rss_bytes: 300 << 20,
          cpu_percent: 1,
          cpu_window_seconds: 60,
          goroutines: 900,
          open_fds: 30,
          threads: 4,
          apps: 0,
        },
        baseline: { apps: 0, peak_rss_mb: 212 },
        checks: [
          {
            name: 'goroutines',
            unit: 'count',
            current: 900,
            baseline: 129,
            limit: 139,
            ok: false,
          },
        ],
      },
    }
    render(<ControlPlaneHealthPage />)
    expect(await screen.findByText('Over tolerance')).toBeTruthy()
    expect(screen.getByText('Footprint')).toBeTruthy()
  })

  it('uses the hero banner and a fix button when a finding needs attention', () => {
    state.health = health([
      {
        id: 'wal_size',
        severity: 'warn',
        value: 300 * (1 << 20),
        threshold: 200 * (1 << 20),
        fix: 'wal_checkpoint',
      },
    ])
    render(<ControlPlaneHealthPage />)
    expect(screen.getByTestId('page-banner-full')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /Checkpoint now/ }))
    expect(mutate).toHaveBeenCalledTimes(1)
  })
})
