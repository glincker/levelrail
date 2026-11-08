import type { Tone } from '@/components/kit'
import type { LoadBalancerConfig } from '../../queries/appLoadBalancer'
import type {
  LiveUpstream,
  LoadBalancerHistory,
} from '../../queries/loadBalancerLive'
import { parseDuration } from './duration'
import { presetById } from './presets'
import { lbT } from '../../lib/lbText'

export interface LbSuggestion {
  id: string
  tone: Tone
  priority: number
  title: string
  detail: string
  actionLabel: string
  apply: ((cfg: LoadBalancerConfig) => LoadBalancerConfig) | null
  href?: 'scale'
}

export interface SuggestionInput {
  config: LoadBalancerConfig
  replicas: number
  upstreams: LiveUpstream[]
  history?: LoadBalancerHistory
  now?: number
}

const FLAP_WINDOW_MS = 10 * 60 * 1000
const FLAP_TRANSITIONS = 3
const TIMEOUT_RATIO = 0.6
const FLAP_FAILS_TARGET = 3

export function p95(values: number[]): number {
  if (values.length === 0) return 0
  const sorted = [...values].sort((a, b) => a - b)
  return (
    sorted[Math.min(sorted.length - 1, Math.ceil(sorted.length * 0.95) - 1)] ??
    0
  )
}

function flapping(
  history: LoadBalancerHistory | undefined,
  now: number,
): boolean {
  return (history?.upstreams ?? []).some(
    (u) =>
      u.transitions.filter(
        (t) => now - new Date(t.at).getTime() <= FLAP_WINDOW_MS,
      ).length >= FLAP_TRANSITIONS,
  )
}

function latencyP95(input: SuggestionInput): number {
  const fromHistory = (input.history?.upstreams ?? []).flatMap((u) =>
    u.checks
      .filter((c) => c.ok && c.latency_ms)
      .map((c) => c.latency_ms as number),
  )
  if (fromHistory.length > 0) return p95(fromHistory)
  return p95(input.upstreams.map((u) => u.latency_ms ?? 0).filter((v) => v > 0))
}

export function computeSuggestions(input: SuggestionInput): LbSuggestion[] {
  const { config, replicas } = input
  const now = input.now ?? Date.now()
  const out: LbSuggestion[] = []
  const health = config.active_health

  if (!health && replicas > 1) {
    const preset = presetById('balanced')?.config
    out.push({
      id: 'no-health',
      tone: 'warning',
      priority: 100,
      title: lbT('suggest.noHealth.title', 'No health check'),
      detail: lbT(
        'suggest.noHealth.detail',
        'A failed replica keeps receiving traffic.',
      ),
      actionLabel: lbT('suggest.noHealth.action', 'Add /healthz check'),
      apply: (cfg) => ({
        ...cfg,
        active_health: preset?.active_health,
        passive_health: cfg.passive_health ?? preset?.passive_health,
      }),
    })
  }
  if (replicas < 2) {
    out.push({
      id: 'single-replica',
      tone: 'info',
      priority: 90,
      title: lbT('suggest.single.title', 'One replica, nothing to balance'),
      detail: lbT('suggest.single.detail', 'Run two replicas for failover.'),
      actionLabel: lbT('suggest.single.action', 'Set replicas'),
      apply: null,
      href: 'scale',
    })
  }
  if (
    health &&
    (health.fails ?? 1) < FLAP_FAILS_TARGET &&
    flapping(input.history, now)
  ) {
    out.push({
      id: 'flapping',
      tone: 'warning',
      priority: 95,
      title: lbT('suggest.flapping.title', 'An upstream is flapping'),
      detail: lbT(
        'suggest.flapping.detail',
        '3 or more state changes in 10 minutes.',
      ),
      actionLabel: lbT('suggest.flapping.action', 'Require 3 failures'),
      apply: (cfg) => ({
        ...cfg,
        active_health: cfg.active_health && {
          ...cfg.active_health,
          fails: Math.max(cfg.active_health.fails ?? 1, FLAP_FAILS_TARGET),
        },
      }),
    })
  }
  if (health) {
    const timeout = parseDuration(health.timeout ?? '5s')
    const interval = parseDuration(health.interval ?? '10s')
    const latency = latencyP95(input) / 1000
    if (latency > 0 && timeout > 0 && latency > timeout * TIMEOUT_RATIO) {
      const next = Math.ceil(latency * 2)
      if (next < interval) {
        out.push({
          id: 'timeout-tight',
          tone: 'warning',
          priority: 85,
          title: lbT('suggest.timeout.title', 'Health timeout is tight'),
          detail: lbT(
            'suggest.timeout.detail',
            'Checks take {{ms}}ms at p95, timeout is {{timeout}}.',
            { ms: Math.round(latency * 1000), timeout: health.timeout ?? '5s' },
          ),
          actionLabel: lbT('suggest.timeout.action', 'Raise to {{next}}s', {
            next,
          }),
          apply: (cfg) => ({
            ...cfg,
            active_health: cfg.active_health && {
              ...cfg.active_health,
              timeout: `${next}s`,
            },
          }),
        })
      }
    }
  }
  if (config.algorithm === 'ip_hash') {
    out.push({
      id: 'iphash-proxy',
      tone: 'info',
      priority: 60,
      title: lbT('suggest.ipHash.title', 'IP hash behind a proxy'),
      detail: lbT(
        'suggest.ipHash.detail',
        'Clients behind a CDN or shared proxy land on one replica.',
      ),
      actionLabel: lbT('suggest.ipHash.action', 'Use sticky cookie'),
      apply: (cfg) => ({
        ...cfg,
        algorithm: 'cookie',
        cookie_name: cfg.cookie_name ?? 'lb',
      }),
    })
  }
  const weights = config.weights ?? []
  if (
    config.algorithm !== 'weighted' &&
    weights.length > 1 &&
    new Set(weights).size > 1
  ) {
    out.push({
      id: 'weights-ignored',
      tone: 'warning',
      priority: 80,
      title: lbT('suggest.weights.title', 'Weights are ignored'),
      detail: lbT(
        'suggest.weights.detail',
        'Only the weighted algorithm uses them.',
      ),
      actionLabel: lbT('suggest.weights.action', 'Use weighted'),
      apply: (cfg) => ({ ...cfg, algorithm: 'weighted' }),
    })
  }
  if (health && !config.passive_health) {
    const preset = presetById('balanced')?.config.passive_health
    out.push({
      id: 'passive-off',
      tone: 'info',
      priority: 50,
      title: lbT('suggest.passive.title', 'Passive checks are off'),
      detail: lbT(
        'suggest.passive.detail',
        'Real request failures could eject a replica sooner.',
      ),
      actionLabel: lbT('suggest.passive.action', 'Enable'),
      apply: (cfg) => ({ ...cfg, passive_health: preset }),
    })
  }
  if (config.algorithm === 'weighted' && !config.slow_start && replicas > 1) {
    out.push({
      id: 'slow-start',
      tone: 'info',
      priority: 40,
      title: lbT('suggest.slowStart.title', 'No slow start'),
      detail: lbT(
        'suggest.slowStart.detail',
        'A new replica gets its full share at once.',
      ),
      actionLabel: lbT('suggest.slowStart.action', 'Ramp over 30s'),
      apply: (cfg) => ({ ...cfg, slow_start: '30s' }),
    })
  }
  if (!config.drain_timeout && replicas > 1) {
    out.push({
      id: 'drain',
      tone: 'info',
      priority: 30,
      title: lbT('suggest.drain.title', 'Deploys cut open streams'),
      detail: lbT('suggest.drain.detail', 'No drain time is set.'),
      actionLabel: lbT('suggest.drain.action', 'Drain for 15s'),
      apply: (cfg) => ({ ...cfg, drain_timeout: '15s' }),
    })
  }
  return out.sort((a, b) => b.priority - a.priority).slice(0, 3)
}
