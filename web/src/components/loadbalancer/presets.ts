import { formFromConfig, type LbFormState } from '../../lib/loadBalancer'
import type { LoadBalancerConfig } from '../../queries/appLoadBalancer'
import { lbT } from '../../lib/lbText'

export interface LbPreset {
  id: string
  name: string
  useWhen: string
  note?: string
  config: LoadBalancerConfig
}

const HEALTH_FAST = {
  path: '/healthz',
  interval: '5s',
  timeout: '2s',
  passes: 2,
  fails: 3,
  expect_status: 200,
}

export const RECOMMENDED_PRESET_ID = 'balanced'

export const LB_PRESETS: LbPreset[] = [
  {
    id: 'balanced',
    get name() {
      return lbT('preset.balanced.name', 'Balanced web app')
    },
    get useWhen() {
      return lbT(
        'preset.balanced.useWhen',
        'A typical stateless web app or API.',
      )
    },
    config: {
      algorithm: 'least_conn',
      active_health: HEALTH_FAST,
      passive_health: { max_fails: 3, fail_duration: '30s' },
      retries: { count: 2, try_duration: '5s' },
      drain_timeout: '15s',
    },
  },
  {
    id: 'sticky',
    get name() {
      return lbT('preset.sticky.name', 'Sticky sessions')
    },
    get useWhen() {
      return lbT(
        'preset.sticky.useWhen',
        'Sessions live in memory on each replica.',
      )
    },
    config: {
      algorithm: 'cookie',
      cookie_name: 'lb',
      active_health: HEALTH_FAST,
      retries: { count: 1 },
      drain_timeout: '30s',
    },
  },
  {
    id: 'websockets',
    get name() {
      return lbT('preset.websockets.name', 'WebSockets and long-lived')
    },
    get useWhen() {
      return lbT(
        'preset.websockets.useWhen',
        'Streams, sockets or long polling.',
      )
    },
    get note() {
      return lbT(
        'preset.websockets.note',
        'Retries are off because upgraded connections cannot be replayed.',
      )
    },
    config: {
      algorithm: 'least_conn',
      active_health: {
        path: '/healthz',
        interval: '10s',
        timeout: '3s',
        passes: 2,
        fails: 3,
      },
      retries: { count: 0 },
      drain_timeout: '60s',
    },
  },
  {
    id: 'cache',
    get name() {
      return lbT('preset.cache.name', 'Cache friendly')
    },
    get useWhen() {
      return lbT(
        'preset.cache.useWhen',
        'Each replica keeps a warm cache per path.',
      )
    },
    config: {
      algorithm: 'uri_hash',
      active_health: { path: '/healthz', interval: '10s', timeout: '3s' },
      retries: { count: 1 },
      drain_timeout: '15s',
    },
  },
  {
    id: 'canary',
    get name() {
      return lbT('preset.canary.name', 'Canary 90/10')
    },
    get useWhen() {
      return lbT(
        'preset.canary.useWhen',
        'Try a new release on a slice of traffic.',
      )
    },
    get note() {
      return lbT(
        'preset.canary.note',
        'Weights follow replica index: with 2 replicas the split is 90/10.',
      )
    },
    config: {
      algorithm: 'weighted',
      weights: [9, 1],
      slow_start: '30s',
      active_health: {
        path: '/healthz',
        interval: '5s',
        timeout: '2s',
        passes: 2,
        fails: 2,
      },
      passive_health: { max_fails: 2, fail_duration: '20s' },
      retries: { count: 1 },
    },
  },
  {
    id: 'bluegreen',
    get name() {
      return lbT('preset.bluegreen.name', 'Blue-green cutover')
    },
    get useWhen() {
      return lbT(
        'preset.bluegreen.useWhen',
        'Swap releases with a fast, clean handover.',
      )
    },
    config: {
      algorithm: 'round_robin',
      active_health: {
        path: '/healthz',
        interval: '3s',
        timeout: '1s',
        passes: 3,
        fails: 2,
      },
      retries: { count: 2, try_duration: '5s' },
      drain_timeout: '30s',
    },
  },
  {
    id: 'apiguard',
    get name() {
      return lbT('preset.apiguard.name', 'Public API guard')
    },
    get useWhen() {
      return lbT('preset.apiguard.useWhen', 'An API exposed to the internet.')
    },
    config: {
      algorithm: 'least_conn',
      rate_limit: { rps: 50, burst: 100 },
      request_timeout: '30s',
      retries: { count: 1 },
    },
  },
]

export function presetById(id: string): LbPreset | undefined {
  return LB_PRESETS.find((p) => p.id === id)
}

export function formFromPreset(
  preset: LbPreset,
  replicas: number,
): LbFormState {
  return formFromConfig(preset.config, replicas)
}
