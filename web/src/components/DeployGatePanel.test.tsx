import { render, screen } from '@testing-library/react'
import i18next from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, it } from 'vitest'
import { GateView } from './DeployGatePanel'
import {
  EMPTY_OVERRIDES,
  WATCH_FIELDS,
  toOverrides,
} from '../lib/deploySafetyForm'
import deploySafetyEn from '../locales/en/deploySafety.json'
import type { DeployGate, GateEvent } from '../types/deployGate'

const testI18n = i18next.createInstance()
void testI18n.use(initReactI18next).init({
  lng: 'en',
  fallbackLng: 'en',
  ns: ['deploySafety'],
  defaultNS: 'deploySafety',
  resources: { en: { deploySafety: deploySafetyEn } },
  interpolation: { escapeValue: false },
})

function event(over: Partial<GateEvent>): GateEvent {
  return {
    phase: 'readiness',
    status: 'passed',
    message: 'first readiness check passed',
    at: '2026-10-10T10:00:00Z',
    ...over,
  }
}

const base: DeployGate = {
  deploy_id: 'dep_4',
  image: 'web:4',
  status: 'failed',
  reason: 'PostDeployErrorRate',
  stage: 'rolled_back',
  terminal: true,
  failed: true,
  superseded: false,
  settings: {
    success_threshold: 2,
    stable_window: '10s',
    drain_timeout: '15s',
    watch_window: '5m0s',
    watch_max_restarts: 2,
    watch_error_rate: 0.2,
    watch_min_requests: 20,
    auto_rollback: true,
    rollback_pins: 3,
    watch_mode: 'auto',
    watch_latency_ratio: 2,
    watch_latency_min_delta: '100ms',
    watch_soft_ticks: 3,
    watch_soft_rollback: false,
  },
  watch: {
    state: 'rolled_back',
    mode: 'auto',
    remaining_seconds: 0,
    window_seconds: 300,
    reason: 'PostDeployErrorRate',
    message: '40% of 200 requests returned 5xx after cutover',
  },
  rollback_pin: null,
  events: [
    event({}),
    event({ phase: 'cutover', message: 'traffic moved to 1 ready replica(s)' }),
    event({
      phase: 'watch',
      status: 'failed',
      reason: 'PostDeployErrorRate',
      message: '40% of 200 requests returned 5xx after cutover',
    }),
    event({ phase: 'rollback', message: 'rolled back to deploy dep_2' }),
  ],
}

function renderGate(gate: DeployGate) {
  return render(
    <I18nextProvider i18n={testI18n}>
      <GateView gate={gate} />
    </I18nextProvider>,
  )
}

describe('GateView', () => {
  it('shows the stage, the rules and every step with its reason', () => {
    renderGate(base)
    expect(screen.getByText('Rolled back')).toBeTruthy()
    expect(screen.getByText(/2 readiness passes in a row and 10s/)).toBeTruthy()
    expect(screen.getByText('Post-deploy watch')).toBeTruthy()
    expect(
      screen.getAllByText(/returned 5xx after cutover \(PostDeployErrorRate\)/)
        .length,
    ).toBeGreaterThan(0)
    expect(screen.getByText('rolled back to deploy dep_2')).toBeTruthy()
  })

  it('caps a long crash-retry history', () => {
    const events = Array.from({ length: 45 }, (_, i) =>
      event({ message: `attempt ${i}` }),
    )
    renderGate({ ...base, events })
    expect(screen.getByText('5 earlier steps are not shown.')).toBeTruthy()
    expect(screen.queryByText('attempt 0')).toBeNull()
    expect(screen.getByText('attempt 44')).toBeTruthy()
  })
})

describe('toOverrides', () => {
  it('turns empty watch fields into inherit and the 5xx percentage into a fraction', () => {
    const out = toOverrides(
      {
        success_threshold: '9',
        stable_window: '',
        drain_timeout: '',
        watch_window: ' 10m ',
        watch_max_restarts: '3',
        watch_error_rate: '25',
        watch_min_requests: '',
        watch_latency_ratio: '2.5',
      },
      EMPTY_OVERRIDES,
      WATCH_FIELDS,
      { watch_mode: 'auto' },
    )
    expect(out).toEqual({
      ...EMPTY_OVERRIDES,
      watch_window: '10m',
      watch_max_restarts: 3,
      watch_error_rate: 0.25,
      watch_latency_ratio: 2.5,
      watch_mode: 'auto',
    })
  })
})
