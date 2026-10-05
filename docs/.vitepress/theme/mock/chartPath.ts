export interface Pt {
  x: number
  y: number
}

export function smoothPath(pts: Pt[]): string {
  if (pts.length < 2) return ''
  let d = `M${pts[0].x.toFixed(1)} ${pts[0].y.toFixed(1)}`
  for (let i = 0; i < pts.length - 1; i++) {
    const p0 = pts[i - 1] ?? pts[i]
    const p1 = pts[i]
    const p2 = pts[i + 1]
    const p3 = pts[i + 2] ?? p2
    const c1x = p1.x + (p2.x - p0.x) / 6
    const c1y = p1.y + (p2.y - p0.y) / 6
    const c2x = p2.x - (p3.x - p1.x) / 6
    const c2y = p2.y - (p3.y - p1.y) / 6
    d += ` C${c1x.toFixed(1)} ${c1y.toFixed(1)} ${c2x.toFixed(1)} ${c2y.toFixed(1)} ${p2.x.toFixed(1)} ${p2.y.toFixed(1)}`
  }
  return d
}

export function toPoints(values: number[], x0: number, x1: number, y0: number, y1: number, min: number, max: number): Pt[] {
  const span = max - min || 1
  const step = values.length > 1 ? (x1 - x0) / (values.length - 1) : 0
  return values.map((v, i) => ({
    x: x0 + i * step,
    y: y1 - ((Math.min(Math.max(v, min), max) - min) / span) * (y1 - y0),
  }))
}
