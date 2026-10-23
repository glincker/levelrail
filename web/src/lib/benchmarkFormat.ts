import type { BenchmarkGroup, BenchmarkResult } from '../queries/benchmarks'

export const GROUP_ORDER: BenchmarkGroup[] = [
  'failover',
  'deploy',
  'rollout',
  'throughput',
]

export function formatMetric(name: string, value: number | null): string {
  if (value === null) return 'n/a'
  const text = Number.isInteger(value)
    ? String(value)
    : value.toFixed(Math.abs(value) < 10 ? 2 : 1)
  if (name.endsWith('_ms')) return `${text} ms`
  if (name.endsWith('rps')) return `${text} req/s`
  return text
}

export function formatPct(value: number | null): string {
  if (value === null) return 'n/a'
  const sign = value > 0 ? '+' : ''
  return `${sign}${value.toFixed(1)}%`
}

export function shortCommit(commit: string, dirty: boolean): string {
  const short = commit.length > 10 ? commit.slice(0, 10) : commit
  return dirty ? `${short} +dirty` : short
}

export function metricKeys(results: BenchmarkResult[]): string[] {
  const set = new Set<string>()
  for (const r of results) {
    for (const k of Object.keys(r.metrics ?? {})) set.add(k)
  }
  return [...set].sort()
}

export function resultsByGroup(
  results: BenchmarkResult[],
  groups: Record<string, BenchmarkGroup>,
): Map<BenchmarkGroup, BenchmarkResult[]> {
  const out = new Map<BenchmarkGroup, BenchmarkResult[]>()
  for (const g of GROUP_ORDER) out.set(g, [])
  for (const r of results) {
    out.get(groups[r.scenario] ?? 'failover')?.push(r)
  }
  return out
}

export function orderedPair<T extends { generated_at: string }>(
  a: T,
  b: T,
): [base: T, head: T] {
  return Date.parse(a.generated_at) <= Date.parse(b.generated_at)
    ? [a, b]
    : [b, a]
}
