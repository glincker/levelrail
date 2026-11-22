export type WireTone = 'ok' | 'warn' | 'bad' | 'off'

/** Dot colour per certificate status; anything unissued reads as pending (amber). */
export const TLS_DOT: Record<string, string> = {
  healthy: 'bg-tone-success-solid',
  expiring_soon: 'bg-tone-warning-solid',
  expired: 'bg-tone-danger-solid',
}

const TLS_TONE: Record<string, WireTone> = {
  healthy: 'ok',
  expiring_soon: 'warn',
  expired: 'bad',
}

/** Missing or unknown certificate state is pending, never healthy. */
export function tlsTone(status: string | undefined): WireTone {
  return (status && TLS_TONE[status]) || 'warn'
}

const RANK: Record<WireTone, number> = { off: 0, ok: 1, warn: 2, bad: 3 }

/** The most urgent tone of a set, so a group line shows its worst member. */
export function worstTone(tones: WireTone[]): WireTone {
  return tones.reduce<WireTone>(
    (acc, t) => (RANK[t] > RANK[acc] ? t : acc),
    'off',
  )
}

/** Solid dot colour per wiring tone. */
export const DOT: Record<WireTone, string> = {
  ok: 'bg-tone-success-solid',
  warn: 'bg-tone-warning-solid',
  bad: 'bg-tone-danger-solid',
  off: 'bg-muted-foreground/40',
}
