import type { StaticRule } from '../../queries/staticRoutes'
import { parseProxy } from './routesLogic'

const MS_PER_SECOND = 1000
const SECONDS_PER_MINUTE = 60
export const SHORT_SHA = 7

const KNOWN_STATES = [
  'succeeded',
  'running',
  'failed',
  'queued',
  'held',
  'canceled',
  'superseded',
  'pending',
  'refused',
  'awaiting_approval',
] as const
export type DeployStateKey = (typeof KNOWN_STATES)[number] | 'unknown'

/** Maps any server status onto a state with an i18n label, so an unknown one never renders a raw key. */
export function deployStateKey(status: string | undefined): DeployStateKey {
  return KNOWN_STATES.find((k) => k === status) ?? 'unknown'
}

/** Build time in whole seconds; null while unfinished or when the timestamps are unusable. */
export function buildSeconds(
  startedAt: string | undefined,
  finishedAt: string | undefined,
): number | null {
  if (!startedAt || !finishedAt) return null
  const ms = new Date(finishedAt).getTime() - new Date(startedAt).getTime()
  if (Number.isNaN(ms) || ms < 0) return null
  return Math.round(ms / MS_PER_SECOND)
}

export function splitDuration(seconds: number): {
  minutes: number
  seconds: number
} {
  return {
    minutes: Math.floor(seconds / SECONDS_PER_MINUTE),
    seconds: seconds % SECONDS_PER_MINUTE,
  }
}

/** owner/repo from a clone URL, falling back to the URL itself when it has no recognisable shape. */
export function repoLabel(url: string): string {
  const trimmed = url.trim().replace(/\.git$/, '')
  const scp = /^[^@/]+@[^:/]+:(.+)$/.exec(trimmed)
  if (scp?.[1]) return scp[1]
  try {
    const path = new URL(trimmed).pathname.replace(/^\/+/, '')
    return path || trimmed
  } catch {
    return trimmed
  }
}

/** Distinct apps that proxy rules point at, in rule order. */
export function proxiedApps(rules: readonly StaticRule[]): string[] {
  const seen = new Set<string>()
  for (const r of rules) {
    const target = parseProxy(r.to)
    if (target?.app) seen.add(target.app)
  }
  return [...seen]
}
