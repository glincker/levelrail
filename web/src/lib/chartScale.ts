// Y-axis scaling and formatting for metric charts: domain computation
// with adaptive precision, tick generation with unique labels, and
// percent/bytes formatters that adapt to the axis range.

// Compute decimals based on the axis max value.
// Max >= 10: 0 decimals; 1 to 10: 1; 0.1 to 1: 2; below 0.1: 3.
function decimalsForMax(max: number): number {
  if (max >= 10) return 0
  if (max >= 1) return 1
  if (max >= 0.1) return 2
  return 3
}

// Format a percent value with adaptive precision based on the axis max.
export function formatPercentWithAdaptivePrecision(
  value: number,
  axisMax: number,
): string {
  if (!Number.isFinite(value)) return '-'
  const decimals = decimalsForMax(axisMax)
  if (value === 0) return '0%'
  const threshold = Math.pow(10, -decimals)
  if (value > 0 && value < threshold) {
    const thresholdStr = threshold.toFixed(decimals)
    return `<${thresholdStr}%`
  }
  const formatted = value.toFixed(decimals)
  return `${formatted}%`
}

// Compute a nice Y domain [min, max] for a chart with the given data.
// Ensures all tick labels are unique after formatting (no dupe 0.0% ticks).
export function computeNiceYDomain(
  values: number[],
  unit: 'percent' | 'bytes' | 'count',
): [number, number] {
  const finiteValues = values.filter(Number.isFinite)
  if (finiteValues.length === 0) {
    return unit === 'percent' ? [0, 0.1] : [0, 100]
  }
  const min = Math.min(...finiteValues)
  const max = Math.max(...finiteValues)

  // For percent, ensure headroom and non-degenerate range.
  if (unit === 'percent') {
    if (min === max) {
      if (max === 0) return [0, 0.1]
      return [Math.max(0, max * 0.9), max * 1.1]
    }
    // 10% headroom above the max.
    const headroom = (max - min) * 0.1
    const domainMax = max + headroom
    const domainMin = Math.max(0, min - headroom)
    return [domainMin, domainMax]
  }

  // For bytes/count, similar headroom logic.
  if (min === max) {
    if (max === 0) return [0, 100]
    return [Math.max(0, max * 0.9), max * 1.1]
  }
  const headroom = (max - min) * 0.1
  const domainMax = max + headroom
  const domainMin = Math.max(0, min - headroom)
  return [domainMin, domainMax]
}

// Generate tick values for the Y axis that will have unique labels
// after formatting, avoiding the "0.0%, 0.0%, 0.0%, 0.0%" problem.
export function generateUniqueTicks(
  domain: [number, number],
  axisMax: number,
  unit: 'percent' | 'bytes' | 'count',
  count: number = 5,
): number[] {
  const [min, max] = domain
  const step = (max - min) / (count - 1)
  const candidates: number[] = []
  for (let i = 0; i < count; i++) {
    candidates.push(min + i * step)
  }

  // Filter to keep only those with unique formatted labels.
  const seen = new Set<string>()
  const unique: number[] = []
  for (const value of candidates) {
    let label: string
    if (unit === 'percent') {
      label = formatPercentWithAdaptivePrecision(value, axisMax)
    } else if (unit === 'bytes') {
      label = formatByteValue(value)
    } else {
      label = Math.round(value).toString()
    }
    if (!seen.has(label)) {
      seen.add(label)
      unique.push(value)
    }
  }

  // If collapsing duplicates left fewer than half the desired count,
  // fall back to every other tick to preserve readability.
  if (unique.length < count / 2 && candidates.length > 0) {
    return candidates.filter((_, i) => i % 2 === 0)
  }
  return unique
}

// Byte formatter, extracted for reuse in tick generation.
function formatByteValue(value: number): string {
  if (!Number.isFinite(value)) return '-'
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB']
  let v = Math.abs(value)
  let unitIndex = 0
  while (v >= 1024 && unitIndex < units.length - 1) {
    v /= 1024
    unitIndex += 1
  }
  const sign = value < 0 ? '-' : ''
  const decimals = unitIndex > 0 && v < 10 ? 1 : 0
  return `${sign}${v.toFixed(decimals)} ${units[unitIndex]}`
}
