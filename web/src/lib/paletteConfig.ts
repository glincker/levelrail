// Palette limits come from the build environment so deployments can tune them; the defaults are the shipped values.
function envInt(raw: unknown, fallback: number): number {
  const n = typeof raw === 'string' ? Number.parseInt(raw, 10) : NaN
  return Number.isFinite(n) && n > 0 ? n : fallback
}

const env = import.meta.env as Record<string, unknown>

/** Round quick-access bubbles beside the search pill (also the Alt+number range). */
export const PALETTE_BUBBLE_MAX = Math.min(
  9,
  envInt(env.VITE_PALETTE_BUBBLE_MAX, 6),
)
/** Distinct destinations kept in the local frecency history. */
export const PALETTE_HISTORY_MAX = envInt(env.VITE_PALETTE_HISTORY_MAX, 200)
/** Recent destinations remembered. */
export const PALETTE_RECENT_MAX = envInt(env.VITE_PALETTE_RECENT_MAX, 5)
/** Recent searches remembered. */
export const PALETTE_SEARCH_MAX = envInt(env.VITE_PALETTE_SEARCH_MAX, 8)
