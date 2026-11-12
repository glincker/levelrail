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

/** Distance from an edge, corner or center at which a released palette eases into place. */
export const PALETTE_SNAP_PX = envInt(env.VITE_PALETTE_SNAP_PX, 24)
/** Keyboard nudge step for the drag grip. */
export const PALETTE_NUDGE_PX = envInt(env.VITE_PALETTE_NUDGE_PX, 24)
/** Size of the grip, the part of the pill that must always stay reachable. */
export const PALETTE_GRIP_PX = envInt(env.VITE_PALETTE_GRIP_PX, 28)
/** Divisor for the share of the pill kept visible besides the grip (3 means a third). */
export const PALETTE_MIN_VISIBLE_DIVISOR = envInt(
  env.VITE_PALETTE_MIN_VISIBLE_DIVISOR,
  3,
)
