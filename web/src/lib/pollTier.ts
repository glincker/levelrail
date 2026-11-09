// Central poll cadences so the dashboard load can be tuned in one place.
// Env only seeds the defaults; hidden tabs are already skipped by react-query.
export type PollTier = 'live' | 'active' | 'steady' | 'idle'

const DEFAULTS: Record<PollTier, number> = {
  live: 5_000,
  active: 15_000,
  steady: 30_000,
  idle: 60_000,
}

const ENV_KEYS: Record<PollTier, string> = {
  live: 'VITE_POLL_LIVE_MS',
  active: 'VITE_POLL_ACTIVE_MS',
  steady: 'VITE_POLL_STEADY_MS',
  idle: 'VITE_POLL_IDLE_MS',
}

export function resolveTierMs(
  tier: PollTier,
  env: Record<string, unknown> = import.meta.env,
): number {
  const raw = Number(env[ENV_KEYS[tier]])
  return Number.isFinite(raw) && raw >= 1_000 ? raw : DEFAULTS[tier]
}

// While the change stream is live, polls of the lists it covers are only a fallback.
let streamHealthy = false

export function setChangeStreamHealthy(healthy: boolean): void {
  streamHealthy = healthy
}

export function resolveStreamFactor(
  env: Record<string, unknown> = import.meta.env,
): number {
  const raw = Number(env.VITE_POLL_STREAM_FACTOR)
  return Number.isFinite(raw) && raw >= 1 ? raw : 4
}

export function pollMs(tier: PollTier): number {
  return resolveTierMs(tier)
}

/** Poll cadence for a query the change stream covers; call it per render, not at import. */
export function topicPollMs(tier: PollTier): number {
  const ms = resolveTierMs(tier)
  return streamHealthy && tier !== 'live' ? ms * resolveStreamFactor() : ms
}
