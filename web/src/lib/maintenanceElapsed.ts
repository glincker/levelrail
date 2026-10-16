// Compact "2h 5m" style span; never negative so a skewed clock shows 0m.
export function formatElapsed(seconds: number): string {
  const total = Math.max(0, Math.floor(seconds))
  const days = Math.floor(total / 86400)
  const hours = Math.floor((total % 86400) / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${minutes}m`
  return `${minutes}m`
}

export function elapsedSince(startedAt: string | undefined, now: number) {
  if (!startedAt) return null
  const start = Date.parse(startedAt)
  if (Number.isNaN(start)) return null
  return Math.max(0, Math.floor((now - start) / 1000))
}
