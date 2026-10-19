import type { PrivateNetworkMapEdge } from '../queries/privateNetworks'

export interface NodePoint {
  id: string
  x: number
  y: number
}

export const MAP_SIZE = 360
const RADIUS = 140

export function circleLayout(ids: string[]): NodePoint[] {
  const center = MAP_SIZE / 2
  if (ids.length === 1) return [{ id: ids[0] ?? '', x: center, y: center }]
  return ids.map((id, i) => {
    const angle = (2 * Math.PI * i) / ids.length - Math.PI / 2
    return {
      id,
      x: Math.round(center + RADIUS * Math.cos(angle)),
      y: Math.round(center + RADIUS * Math.sin(angle)),
    }
  })
}

export type EdgeTone = 'allowed' | 'blocked' | 'drift'

export function edgeTone(e: PrivateNetworkMapEdge): EdgeTone {
  if (e.fix === 'reapply') return 'drift'
  return e.allowed ? 'allowed' : 'blocked'
}

export function edgeKey(e: PrivateNetworkMapEdge): string {
  return `${e.a}:${e.b}`
}
