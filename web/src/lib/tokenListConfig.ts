// Tunables come from build-time env with the same defaults the server list uses.
const num = (raw: unknown, fallback: number): number => {
  const n = Number(raw)
  return Number.isFinite(n) && n > 0 ? n : fallback
}

const env = import.meta.env

export const TOKEN_PAGE_SIZE = num(env.VITE_TOKENS_PAGE_SIZE, 100)
export const TOKEN_GROUP_PAGE_SIZE = num(env.VITE_TOKENS_GROUP_PAGE_SIZE, 50)
// Grouping turns on by default once any one name has this many tokens.
export const TOKEN_GROUP_MIN = num(env.VITE_TOKENS_GROUP_MIN, 5)
// A group larger than this starts collapsed.
export const TOKEN_GROUP_COLLAPSE = num(env.VITE_TOKENS_GROUP_COLLAPSE, 5)
export const TOKEN_ROOT_WARN_MIN = num(env.VITE_TOKENS_ROOT_WARN_MIN, 5)
// Revoking more than this many tokens asks for the count to be typed.
export const TOKEN_TYPED_CONFIRM_ABOVE = num(
  env.VITE_TOKENS_TYPED_CONFIRM_ABOVE,
  10,
)
export const TOKEN_PRUNE_DAYS = num(env.VITE_TOKENS_PRUNE_DAYS, 30)
export const TOKEN_RECENT_USE_MS =
  num(env.VITE_TOKENS_RECENT_USE_HOURS, 24) * 60 * 60 * 1000
