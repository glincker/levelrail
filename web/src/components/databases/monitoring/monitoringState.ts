import type { MetricPoint } from '../../../types/metrics'

export type MonitoringState =
  'provisioning' | 'failed' | 'stopped' | 'restarting' | 'noData' | 'live'

export function restartWindowMs(
  env: Record<string, unknown> = import.meta.env,
): number {
  const raw = Number(env.VITE_MONITORING_RESTART_WINDOW_MS)
  return Number.isFinite(raw) && raw >= 1_000 ? raw : 300_000
}

// The restart counter only grows, so a step inside the window means the
// container restarted recently.
export function restartedRecently(
  points: readonly MetricPoint[] | undefined,
  now: number,
  windowMs: number,
): boolean {
  if (!points) return false
  for (let i = 1; i < points.length; i += 1) {
    const cur = points[i]
    const prev = points[i - 1]
    if (!cur || !prev) continue
    const t = Date.parse(cur.timestamp)
    if (cur.value > prev.value && now - t <= windowMs) return true
  }
  return false
}

export interface StateInput {
  suspended?: boolean
  provisionState?: 'pending' | 'failed'
  restarting: boolean
  hasData: boolean
  isLoading: boolean
}

export function resolveMonitoringState(i: StateInput): MonitoringState {
  if (i.provisionState === 'failed') return 'failed'
  if (i.provisionState) return 'provisioning'
  if (i.suspended) return 'stopped'
  if (i.isLoading) return 'live'
  if (!i.hasData) return 'noData'
  return i.restarting ? 'restarting' : 'live'
}

/** States where the compute is not serving, so charts are hatched, not drawn. */
export function isInactive(
  state: MonitoringState,
): state is 'provisioning' | 'failed' | 'stopped' {
  return state === 'provisioning' || state === 'failed' || state === 'stopped'
}
