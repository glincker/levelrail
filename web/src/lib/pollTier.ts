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

export function pollMs(tier: PollTier): number {
  return resolveTierMs(tier)
}
