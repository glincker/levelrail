import type { SplitVariant } from '../queries/splitTesting'

export const SPLIT_TOTAL = 100

/** productionShare is what production keeps once the other arms take their sliders. */
export function productionShare(others: number[]): number {
  return SPLIT_TOTAL - others.reduce((a, b) => a + b, 0)
}

/** weightsPayload maps every variant to its weight, with production taking the remainder. */
export function weightsPayload(
  variants: SplitVariant[],
  edits: Record<string, number>,
): Record<string, number> | null {
  const others = variants.filter((v) => v.kind !== 'production')
  const values = others.map((v) => edits[v.id] ?? v.weight)
  const prod = productionShare(values)
  if (prod < 0 || values.some((w) => w < 0 || !Number.isInteger(w))) return null
  const out: Record<string, number> = {}
  for (const v of variants) {
    out[v.id] = v.kind === 'production' ? prod : (edits[v.id] ?? v.weight)
  }
  return out
}

/** weightsDirty reports whether the edits differ from the saved weights. */
export function weightsDirty(
  variants: SplitVariant[],
  edits: Record<string, number>,
): boolean {
  return variants.some(
    (v) => v.kind !== 'production' && (edits[v.id] ?? v.weight) !== v.weight,
  )
}
