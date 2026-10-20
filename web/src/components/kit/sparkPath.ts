export interface SparkPoint {
  x: number
  y: number
}

export function sparkPoints(
  values: number[],
  width: number,
  height: number,
  pad = 2,
): SparkPoint[] {
  const clean = values.filter((v) => Number.isFinite(v))
  if (clean.length === 0) return []
  const min = Math.min(...clean)
  const max = Math.max(...clean)
  const span = max - min
  const innerW = Math.max(width - pad * 2, 0)
  const innerH = Math.max(height - pad * 2, 0)
  if (clean.length === 1) {
    return [{ x: width / 2, y: height / 2 }]
  }
  return clean.map((v, i) => ({
    x: pad + (i / (clean.length - 1)) * innerW,
    y: span === 0 ? height / 2 : pad + (1 - (v - min) / span) * innerH,
  }))
}

const r = (n: number) => Math.round(n * 100) / 100

// Catmull-Rom converted to cubic beziers, y clamped so overshoot never leaves the box.
export function smoothPath(points: SparkPoint[], height = Infinity): string {
  const first = points[0]
  if (!first) return ''
  let d = `M${r(first.x)} ${r(first.y)}`
  const clampY = (y: number) => Math.min(Math.max(y, 0), height)
  for (let i = 0; i < points.length - 1; i++) {
    const p1 = points[i]
    const p2 = points[i + 1]
    if (!p1 || !p2) continue
    const p0 = points[i - 1] ?? p1
    const p3 = points[i + 2] ?? p2
    const c1x = p1.x + (p2.x - p0.x) / 6
    const c1y = clampY(p1.y + (p2.y - p0.y) / 6)
    const c2x = p2.x - (p3.x - p1.x) / 6
    const c2y = clampY(p2.y - (p3.y - p1.y) / 6)
    d += ` C${r(c1x)} ${r(c1y)} ${r(c2x)} ${r(c2y)} ${r(p2.x)} ${r(p2.y)}`
  }
  return d
}

export function areaPath(
  line: string,
  points: SparkPoint[],
  height: number,
): string {
  const first = points[0]
  const last = points[points.length - 1]
  if (!first || !last || points.length < 2) return ''
  return `${line} L${r(last.x)} ${height} L${r(first.x)} ${height} Z`
}
