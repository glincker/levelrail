// Whole seconds until the next check, or null when unknown or already due.
export function secondsUntil(iso: string | undefined, now: number) {
  if (!iso) return null
  const ms = Date.parse(iso) - now
  if (Number.isNaN(ms) || ms <= 0) return null
  return Math.ceil(ms / 1000)
}
