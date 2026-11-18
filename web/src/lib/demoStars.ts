/** formatStars renders 1234 as 1.2k; an absent or invalid count yields null so the button shows no number. */
export function formatStars(n: number | undefined): string | null {
  if (n === undefined || !Number.isFinite(n) || n < 0) return null
  const v = Math.floor(n)
  if (v < 1000) return String(v)
  if (v < 1_000_000) return `${trim(v / 1000)}k`
  return `${trim(v / 1_000_000)}M`
}

// Truncate, never round up, so 1999 reads 1.9k instead of 2.0k.
function trim(x: number): string {
  return (Math.floor(x * 10) / 10).toString()
}
