import {
  HEADER_HEIGHT,
  ROW_HEIGHT,
  routeEdge,
  type MapEdge,
  type MapLayout,
  type MapTable,
} from './schemaMapLayout'

export function escapeXml(s: string): string {
  return s
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&apos;')
}

// Standalone SVG with fixed light colors so the file reads anywhere. Every
// name and type from the database is escaped.
export function buildSchemaSvg(
  layout: MapLayout,
  tables: MapTable[],
  edges: MapEdge[],
  moreLabel: (n: number) => string,
): string {
  const byId = new Map(tables.map((t) => [t.id, t]))
  const placed = new Map(layout.nodes.map((n) => [n.id, n]))
  const out: string[] = [
    `<svg xmlns="http://www.w3.org/2000/svg" width="${layout.width}" height="${layout.height}" viewBox="0 0 ${layout.width} ${layout.height}" font-family="ui-sans-serif, system-ui, sans-serif" font-size="12">`,
    `<rect width="100%" height="100%" fill="white"/>`,
  ]
  for (const e of edges) {
    const a = placed.get(e.from)
    const b = placed.get(e.to)
    if (!a || !b) continue
    const g = routeEdge(a, b, e.fromColumn)
    out.push(
      `<path d="${g.d}" fill="none" stroke="slategray" stroke-width="1.5"/>`,
    )
  }
  for (const n of layout.nodes) {
    const t = byId.get(n.id)
    if (!t) continue
    out.push(
      `<g><rect x="${n.x}" y="${n.y}" width="${n.w}" height="${n.h}" rx="6" fill="whitesmoke" stroke="lightgray"/>`,
      `<text x="${n.x + 10}" y="${n.y + 18}" font-weight="600" fill="black">${escapeXml(`${t.schema}.${t.name}`)}</text>`,
    )
    n.shown.forEach((c, i) => {
      const y = n.y + HEADER_HEIGHT + i * ROW_HEIGHT + 13
      out.push(
        `<text x="${n.x + 10}" y="${y}" fill="black"${c.primaryKey ? ' font-weight="700"' : ''}>${escapeXml(c.name)}</text>`,
        `<text x="${n.x + n.w - 10}" y="${y}" text-anchor="end" fill="slategray">${escapeXml(c.type)}</text>`,
      )
    })
    if (n.more > 0) {
      const y = n.y + HEADER_HEIGHT + n.shown.length * ROW_HEIGHT + 13
      out.push(
        `<text x="${n.x + 10}" y="${y}" fill="slategray">${escapeXml(moreLabel(n.more))}</text>`,
      )
    }
    out.push('</g>')
  }
  out.push('</svg>')
  return out.join('\n')
}
