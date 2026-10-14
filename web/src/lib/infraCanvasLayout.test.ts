import { describe, expect, it } from 'vitest'
import {
  defaultPositions,
  fitViewport,
  healthOf,
  mergePositions,
  nodeIntersects,
  parseSavedPositions,
  visibleWorldRect,
  zoomAt,
} from './infraCanvasLayout'
import type { ProjectTopologyGraph } from '../types/projectTopology'

const graph: ProjectTopologyGraph = {
  nodes: [
    { id: 'app:web', kind: 'app', label: 'web' },
    { id: 'database:main', kind: 'database', label: 'main' },
    { id: 'domain:a.example.com', kind: 'domain', label: 'a.example.com' },
  ],
  edges: [],
}

describe('infra canvas layout', () => {
  it('places each kind in its own column, domains left of apps', () => {
    const p = defaultPositions(graph)
    expect(p['domain:a.example.com']!.x).toBeLessThan(p['app:web']!.x)
    expect(p['app:web']!.x).toBeLessThan(p['database:main']!.x)
  })

  it('keeps saved positions and defaults the rest', () => {
    const merged = mergePositions(graph, { 'app:web': { x: 5, y: 6 } })
    expect(merged['app:web']).toEqual({ x: 5, y: 6 })
    expect(merged['database:main']).toBeDefined()
  })

  it('ignores corrupt saved JSON', () => {
    expect(parseSavedPositions('{nope')).toEqual({})
    expect(parseSavedPositions('{"a":{"x":1,"y":2},"b":"x"}')).toEqual({
      a: { x: 1, y: 2 },
    })
  })

  it('culls nodes outside the viewport', () => {
    const rect = visibleWorldRect(
      { x: 0, y: 0, scale: 1 },
      { width: 400, height: 300 },
      0,
    )
    expect(nodeIntersects({ x: 10, y: 10 }, rect)).toBe(true)
    expect(nodeIntersects({ x: 5000, y: 10 }, rect)).toBe(false)
  })

  it('clamps zoom to the maximum scale', () => {
    const z = zoomAt({ x: 0, y: 0, scale: 1 }, 100, { x: 50, y: 50 })
    expect(z.scale).toBe(2.5)
  })

  it('fits positions inside the container', () => {
    const v = fitViewport(defaultPositions(graph), { width: 800, height: 600 })
    expect(v.scale).toBeLessThanOrEqual(1)
  })

  it('maps status to health', () => {
    const base = { id: 'x', kind: 'app' as const, label: 'x' }
    expect(
      healthOf({ ...base, status: { label: 'Healthy', variant: 'success' } }),
    ).toBe('healthy')
    expect(
      healthOf({ ...base, status: { label: 'Reconciling', variant: 'muted' } }),
    ).toBe('deploying')
    expect(
      healthOf({
        ...base,
        status: { label: 'Attention needed', variant: 'destructive' },
      }),
    ).toBe('failed')
    expect(healthOf(base)).toBe('idle')
  })
})
