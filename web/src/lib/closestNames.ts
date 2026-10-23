// Edit distance for "did you mean" hints; names are short so O(n*m) is fine.
function distance(a: string, b: string): number {
  let prev = Array.from({ length: b.length + 1 }, (_, i) => i)
  for (let i = 1; i <= a.length; i++) {
    const row = [i]
    for (let j = 1; j <= b.length; j++) {
      const cost = a[i - 1] === b[j - 1] ? 0 : 1
      row.push(
        Math.min(
          (prev[j] ?? 0) + 1,
          (row[j - 1] ?? 0) + 1,
          (prev[j - 1] ?? 0) + cost,
        ),
      )
    }
    prev = row
  }
  return prev[b.length] ?? 0
}

/** Up to `limit` candidates that look like `target`: substring hits or a small edit distance. */
export function closestNames(
  target: string,
  candidates: readonly string[],
  limit = 3,
): string[] {
  const t = target.toLowerCase()
  if (!t) return []
  const maxDistance = Math.max(2, Math.floor(t.length / 2))
  return candidates
    .map((name) => {
      const n = name.toLowerCase()
      const near = n.includes(t) || t.includes(n)
      return { name, score: near ? 0 : distance(t, n) }
    })
    .filter((c) => c.score <= maxDistance && c.name.toLowerCase() !== t)
    .sort((a, b) => a.score - b.score || a.name.localeCompare(b.name))
    .slice(0, limit)
    .map((c) => c.name)
}
