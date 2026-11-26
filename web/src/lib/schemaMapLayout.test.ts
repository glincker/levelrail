import { describe, expect, it } from 'vitest'
import { buildSchemaSvg, escapeXml } from './schemaMapExport'
import { layoutSchemaMap, type MapEdge, type MapTable } from './schemaMapLayout'

function table(name: string, cols = 3): MapTable {
  return {
    id: `public.${name}`,
    schema: 'public',
    name,
    columns: Array.from({ length: cols }, (_, i) => ({
      name: i === 0 ? 'id' : `c${i}`,
      type: 'int',
      primaryKey: i === 0,
    })),
  }
}

const tables = [
  table('users'),
  table('orders', 12),
  table('items'),
  table('products'),
  table('audit'),
  table('flags'),
]
const edges: MapEdge[] = [
  { id: 'a', from: 'public.orders', to: 'public.users', fromColumn: 'c1' },
  { id: 'b', from: 'public.items', to: 'public.orders', fromColumn: 'c1' },
  { id: 'c', from: 'public.items', to: 'public.products', fromColumn: 'c2' },
]

describe('layoutSchemaMap', () => {
  it('is deterministic regardless of input order', () => {
    const a = layoutSchemaMap(tables, edges)
    const b = layoutSchemaMap([...tables].reverse(), [...edges].reverse())
    const key = (l: typeof a) =>
      JSON.stringify([...l.nodes].sort((x, y) => (x.id < y.id ? -1 : 1)))
    expect(key(a)).toBe(key(b))
  })

  it('never overlaps nodes', () => {
    const { nodes } = layoutSchemaMap(tables, edges)
    expect(nodes).toHaveLength(tables.length)
    for (let i = 0; i < nodes.length; i++) {
      for (let j = i + 1; j < nodes.length; j++) {
        const p = nodes[i]
        const q = nodes[j]
        const apart =
          p.x + p.w <= q.x ||
          q.x + q.w <= p.x ||
          p.y + p.h <= q.y ||
          q.y + q.h <= p.y
        expect(apart).toBe(true)
      }
    }
  })

  it('caps columns and keeps primary keys', () => {
    const n = layoutSchemaMap(tables, edges).nodes.find(
      (x) => x.id === 'public.orders',
    )
    expect(n?.shown).toHaveLength(8)
    expect(n?.more).toBe(4)
    expect(n?.shown[0].primaryKey).toBe(true)
  })
})

describe('buildSchemaSvg', () => {
  it('escapes names and types', () => {
    const evil: MapTable = {
      id: 's.<x>',
      schema: 's',
      name: '<x>&"',
      columns: [{ name: '<script>', type: "a'b", primaryKey: true }],
    }
    const layout = layoutSchemaMap([evil], [])
    const svg = buildSchemaSvg(layout, [evil], [], (n) => `+${n}`)
    expect(svg).not.toContain('<script>')
    expect(svg).toContain('&lt;script&gt;')
    expect(svg).toContain('s.&lt;x&gt;&amp;&quot;')
    expect(escapeXml("'")).toBe('&apos;')
  })
})
