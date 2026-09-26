import type { Tone } from '@/components/kit'
import type { LoadBalancerAlgorithm } from '../../queries/appLoadBalancer'
import type {
  LiveUpstream,
  UpstreamHistory,
} from '../../queries/loadBalancerLive'

export type PoolLevel = 'idle' | 'balancing' | 'degraded' | 'down'

export interface PoolRollup {
  level: PoolLevel
  healthy: number
  total: number
  label: string
  tone: Tone
  allDown: boolean
}

export function isInRotation(u: LiveUpstream): boolean {
  return u.healthy && (u.admin_state ?? 'active') === 'active'
}

// Worst-of rollup: every member healthy is Balancing, none healthy is Down
// (the proxy then tries members anyway), anything between is Degraded.
export function rollupPool(upstreams: LiveUpstream[]): PoolRollup {
  const total = upstreams.length
  const healthy = upstreams.filter(isInRotation).length
  if (total === 0) {
    return {
      level: 'idle',
      healthy,
      total,
      label: 'No upstreams',
      tone: 'neutral',
      allDown: false,
    }
  }
  if (healthy === total) {
    return {
      level: 'balancing',
      healthy,
      total,
      label: 'Balancing',
      tone: 'success',
      allDown: false,
    }
  }
  if (healthy === 0) {
    return {
      level: 'down',
      healthy,
      total,
      label: 'Down',
      tone: 'danger',
      allDown: true,
    }
  }
  return {
    level: 'degraded',
    healthy,
    total,
    label: 'Degraded',
    tone: 'warning',
    allDown: false,
  }
}

export interface UpstreamView {
  tone: Tone
  label: string
  reason: string
  glyph: 'ok' | 'warn' | 'down' | 'unknown' | 'off'
}

export function upstreamView(u: LiveUpstream): UpstreamView {
  const admin = u.admin_state ?? 'active'
  if (admin === 'disabled') {
    return {
      tone: 'neutral',
      label: 'Disabled',
      reason: 'Taken out of rotation by an operator',
      glyph: 'off',
    }
  }
  if (admin === 'draining' || u.state === 'draining') {
    return {
      tone: 'warning',
      label: 'Draining',
      reason: u.reason || 'Finishing open requests, no new ones',
      glyph: 'warn',
    }
  }
  if (u.state === 'unhealthy' || (!u.healthy && u.state !== 'unknown')) {
    return {
      tone: 'danger',
      label: 'Down',
      reason: u.reason || 'Failing health checks',
      glyph: 'down',
    }
  }
  if (u.state === 'unknown') {
    return {
      tone: 'neutral',
      label: 'Unknown',
      reason: u.reason || 'Not checked yet',
      glyph: 'unknown',
    }
  }
  return {
    tone: 'success',
    label: 'Healthy',
    reason: u.reason || 'Passing health checks',
    glyph: 'ok',
  }
}

export function upstreamShares(
  upstreams: LiveUpstream[],
  algorithm: LoadBalancerAlgorithm,
): Map<string, number> {
  const live = upstreams.filter(isInRotation)
  const weightOf = (u: LiveUpstream) =>
    algorithm === 'weighted' && u.weight > 0 ? u.weight : 1
  const total = live.reduce((sum, u) => sum + weightOf(u), 0)
  const out = new Map<string, number>()
  for (const u of upstreams) {
    out.set(
      u.id,
      total > 0 && isInRotation(u)
        ? Math.round((weightOf(u) / total) * 100)
        : 0,
    )
  }
  return out
}

export function medianLatency(upstreams: LiveUpstream[]): number | null {
  const values = upstreams
    .filter(
      (u) => u.healthy && typeof u.latency_ms === 'number' && u.latency_ms > 0,
    )
    .map((u) => u.latency_ms as number)
    .sort((a, b) => a - b)
  if (values.length === 0) return null
  const mid = Math.floor(values.length / 2)
  return values.length % 2
    ? (values[mid] ?? null)
    : Math.round(((values[mid - 1] ?? 0) + (values[mid] ?? 0)) / 2)
}

export function changedAt(
  u: LiveUpstream,
  h: UpstreamHistory | undefined,
): string | undefined {
  return u.last_changed_at ?? h?.transitions.at(-1)?.at
}
