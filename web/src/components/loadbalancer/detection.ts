import type { LbFormState } from '../../lib/loadBalancer'
import { formatSeconds, parseDuration } from './duration'

export interface DetectionTimes {
  detectSeconds: number
  recoverSeconds: number
  text: string
}

const count = (raw: string): number => {
  const n = Number.parseInt(raw, 10)
  return Number.isFinite(n) && n > 0 ? n : 1
}

// Mirrors the server defaults: interval 10s, one failure marks a replica down,
// one pass brings it back.
export function detectionTimes(form: LbFormState): DetectionTimes | null {
  if (!form.healthEnabled) return null
  const interval = parseDuration(form.healthInterval)
  const seconds = Number.isNaN(interval) ? 10 : interval
  const detectSeconds = seconds * count(form.healthFails)
  const recoverSeconds = seconds * count(form.healthPasses)
  return {
    detectSeconds,
    recoverSeconds,
    text: `Down in about ${formatSeconds(detectSeconds)}, back in about ${formatSeconds(recoverSeconds)}`,
  }
}

export function weightShares(weights: number[]): number[] {
  const total = weights.reduce((a, b) => a + b, 0)
  if (total <= 0) return weights.map(() => 0)
  const raw = weights.map((w) => (w / total) * 100)
  const floors = raw.map(Math.floor)
  let left = 100 - floors.reduce((a, b) => a + b, 0)
  const order = raw
    .map((r, i) => ({ i, frac: r - Math.floor(r) }))
    .sort((a, b) => b.frac - a.frac || a.i - b.i)
  for (const { i } of order) {
    if (left <= 0) break
    floors[i] = (floors[i] ?? 0) + 1
    left -= 1
  }
  return floors
}
