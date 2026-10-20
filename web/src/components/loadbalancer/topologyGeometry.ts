import type { LiveUpstream } from '../../queries/loadBalancerLive'

export const VIEW_W = 520
export const NODE_H = 46
export const NODE_GAP = 12
export const PAD = 16
export const MAX_NODES = 8
export const CLIENT = { x: 8, w: 84 }
export const PROXY = { x: 186, w: 100 }
export const UP = { x: 336, w: 176 }

export function viewHeight(count: number, overflow: boolean): number {
  const n = Math.max(count, 1)
  const extra = overflow ? 20 : 0
  return Math.max(150, PAD * 2 + n * NODE_H + (n - 1) * NODE_GAP + extra)
}

export function nodeY(index: number): number {
  return PAD + index * (NODE_H + NODE_GAP)
}

// Density follows the share of open connections; falls back to an even split.
export function trafficShares(upstreams: LiveUpstream[]): number[] {
  const total = upstreams.reduce((s, u) => s + u.active_connections, 0)
  if (total === 0) return upstreams.map(() => 1 / Math.max(upstreams.length, 1))
  return upstreams.map((u) => u.active_connections / total)
}

// Faster dashes for lower latency, clamped so motion stays readable.
export function flowSeconds(latencyMs: number | undefined): number {
  const ms = latencyMs && latencyMs > 0 ? latencyMs : 200
  return Math.min(2.4, Math.max(0.6, 0.6 + ms / 400))
}
