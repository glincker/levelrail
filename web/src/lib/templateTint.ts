export type MonogramTone = 'info' | 'success' | 'warning' | 'accent'

const TONES: readonly MonogramTone[] = ['info', 'success', 'warning', 'accent']

// Literal class strings so Tailwind's scanner keeps them.
export const MONOGRAM_TONE_CLASS: Record<MonogramTone, string> = {
  info: 'bg-tone-info-soft text-tone-info',
  success: 'bg-tone-success-soft text-tone-success',
  warning: 'bg-tone-warning-soft text-tone-warning',
  accent: 'bg-tone-accent-soft text-tone-accent',
}

/** monogramTone picks a stable tone for a template id (FNV-1a hash). */
export function monogramTone(id: string): MonogramTone {
  let hash = 0x811c9dc5
  for (let i = 0; i < id.length; i++) {
    hash ^= id.charCodeAt(i)
    hash = Math.imul(hash, 0x01000193) >>> 0
  }
  return TONES[hash % TONES.length] as MonogramTone
}
