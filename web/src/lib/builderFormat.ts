import type { StatusState, Tone } from '@/components/kit/tone'
import type {
  Builder,
  BuilderBuild,
  BuilderPressure,
  BuilderStatus,
} from '../queries/builders'
import { capacityPercent } from './buildRunnerFormat'

const STATE: Record<BuilderStatus, StatusState> = {
  ready: 'ready',
  busy: 'building',
  paused: 'paused',
  offline: 'offline',
  pressure: 'degraded',
  draining: 'draining',
}

export function builderState(status: string): StatusState {
  return status in STATE ? STATE[status as BuilderStatus] : 'pending'
}

const PRESSURE_TONE: Record<BuilderPressure, Tone> = {
  ok: 'success',
  warn: 'warning',
  critical: 'danger',
  unknown: 'neutral',
}

export function pressureTone(pressure: string): Tone {
  return pressure in PRESSURE_TONE
    ? PRESSURE_TONE[pressure as BuilderPressure]
    : 'neutral'
}

export function isBuilderPressure(p: string): p is BuilderPressure {
  return p in PRESSURE_TONE
}

export function loadPercent(used: number, total: number): number {
  return capacityPercent(used, total)
}

// "1m 05s" style for a millisecond duration; zero reads as a dash.
export function formatDurationMs(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '-'
  const s = Math.round(ms / 1000)
  if (s < 1) return `${Math.round(ms)}ms`
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  return `${m}m ${String(s % 60).padStart(2, '0')}s`
}

// Oldest first so the sparkline reads left to right in time.
export function durationSeries(builds: readonly BuilderBuild[]): number[] {
  return [...builds]
    .sort((a, b) => a.started_at.localeCompare(b.started_at))
    .map((b) => b.duration_ms)
}

export function durationBarPercent(ms: number, max: number): number {
  if (max <= 0) return 0
  return Math.max(4, Math.min(100, Math.round((ms / max) * 100)))
}

export function osArchLabel(os: string, arch: string): string {
  return [os, arch].filter(Boolean).join(' / ')
}

// A builder that was not in the list when the token was minted and has joined as personal.
export function findJoinedBuilder(
  builders: readonly Builder[],
  baseline: readonly string[],
): Builder | undefined {
  return builders.find((b) => !baseline.includes(b.id) && b.tier === 'personal')
}
