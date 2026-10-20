import { formFromConfig, type LbFormState } from '../../lib/loadBalancer'
import type { LoadBalancerConfig } from '../../queries/appLoadBalancer'

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
    name: 'Balanced web app',
    useWhen: 'A typical stateless web app or API.',
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
    name: 'Sticky sessions',
    useWhen: 'Sessions live in memory on each replica.',
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
    name: 'WebSockets and long-lived',
    useWhen: 'Streams, sockets or long polling.',
    note: 'Retries are off because upgraded connections cannot be replayed.',
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
    name: 'Cache friendly',
    useWhen: 'Each replica keeps a warm cache per path.',
    config: {
      algorithm: 'uri_hash',
      active_health: { path: '/healthz', interval: '10s', timeout: '3s' },
      retries: { count: 1 },
      drain_timeout: '15s',
    },
  },
  {
    id: 'canary',
    name: 'Canary 90/10',
    useWhen: 'Try a new release on a slice of traffic.',
    note: 'Weights follow replica index: with 2 replicas the split is 90/10.',
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
    name: 'Blue-green cutover',
    useWhen: 'Swap releases with a fast, clean handover.',
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
    name: 'Public API guard',
    useWhen: 'An API exposed to the internet.',
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
