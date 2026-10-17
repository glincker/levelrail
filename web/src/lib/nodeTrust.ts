import type { NodeSecurityEvent } from '../types/nodeTrust'

// "ab12cd" -> "AB:12:CD", the grouping every other tool prints a
// fingerprint in, so an operator can compare by eye.
export function groupFingerprint(fp?: string): string {
  if (!fp) return ''
  const pairs = fp.match(/.{1,2}/g) ?? []
  return pairs.join(':').toUpperCase()
}

const KINDS = [
  'enroll',
  'reenroll',
  'ca_pin',
  'connect',
  'cert_issued',
  'renew_failed',
  'revoke',
  'renew_requested',
] as const

export type EventKindKey = (typeof KINDS)[number] | 'other'

// Kinds with their own sentence; anything else gets the generic wording.
export function eventKindKey(e: NodeSecurityEvent): EventKindKey {
  return KINDS.find((k) => k === e.kind) ?? 'other'
}

// Unix seconds from the event detail, as a Date.
export function unixToDate(sec?: number): Date | undefined {
  return sec ? new Date(sec * 1000) : undefined
}
