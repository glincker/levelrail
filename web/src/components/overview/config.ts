function numberFromEnv(raw: unknown, fallback: number): number {
  const n = typeof raw === 'string' ? Number(raw) : NaN
  return Number.isFinite(n) && n > 0 ? n : fallback
}

/** Error share (4xx plus 5xx, 0 to 1) above which the tile turns danger. */
export const ERROR_RATE_DANGER = numberFromEnv(
  import.meta.env.VITE_OVERVIEW_ERROR_RATE_DANGER,
  0.05,
)

/** Memory usage share of the limit above which "raise the limit" shows. */
export const MEMORY_HIGH_RATIO = numberFromEnv(
  import.meta.env.VITE_OVERVIEW_MEMORY_HIGH_RATIO,
  0.8,
)

/** Requests-series window, in minutes; the previous window is the same length. */
export const TRAFFIC_WINDOW_MINUTES = numberFromEnv(
  import.meta.env.VITE_OVERVIEW_TRAFFIC_WINDOW_MINUTES,
  30,
)

export const TRAFFIC_REFETCH_MS = 15_000
