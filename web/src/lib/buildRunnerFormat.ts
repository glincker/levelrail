import type { StatusState } from '@/components/kit/tone'
import type { RunnerStatus } from '../queries/buildRunners'

// Slot use as a whole percent; a runner with no slots reads as empty, never NaN.
export function capacityPercent(used: number, total: number): number {
  if (!Number.isFinite(used) || !Number.isFinite(total) || total <= 0) return 0
  return Math.min(100, Math.max(0, Math.round((used / total) * 100)))
}

export const RUNNER_STATE: Record<RunnerStatus, StatusState> = {
  ready: 'ready',
  busy: 'building',
  pressure: 'degraded',
  draining: 'draining',
  offline: 'offline',
}

export function runnerState(status: string): StatusState {
  return status in RUNNER_STATE
    ? RUNNER_STATE[status as RunnerStatus]
    : 'pending'
}

// Queue wait for the run header: whole seconds under a minute, then minutes.
export function formatWaitMs(ms: number): string {
  const seconds = Math.round(ms / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  const rest = seconds % 60
  return rest === 0 ? `${minutes}m` : `${minutes}m ${rest}s`
}

// Labels are key or key=value; the form accepts them comma or space separated.
export function parseLabels(raw: string): string[] {
  return Array.from(
    new Set(
      raw
        .split(/[\s,]+/)
        .map((l) => l.trim())
        .filter(Boolean),
    ),
  )
}
