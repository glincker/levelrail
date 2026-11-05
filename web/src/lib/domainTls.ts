export type TlsTone = 'error' | 'warn' | 'ok'

// Days left under which a certificate reads as expiring soon.
export const TLS_WARN_DAYS = (() => {
  const n = Number(import.meta.env.VITE_DOMAIN_TLS_WARN_DAYS)
  return Number.isFinite(n) && n > 0 ? n : 14
})()

export function tlsTone(daysLeft: number, warnDays = TLS_WARN_DAYS): TlsTone {
  if (daysLeft < 0) return 'error'
  return daysLeft < warnDays ? 'warn' : 'ok'
}

export interface TlsDaysText {
  key: 'page.cert.daysLeft' | 'page.cert.expiredAgo' | 'page.cert.expiresToday'
  count: number
}

// Maps the API's signed day count to a translation key and a positive count.
export function tlsDaysText(daysLeft: number): TlsDaysText {
  if (daysLeft < 0) return { key: 'page.cert.expiredAgo', count: -daysLeft }
  if (daysLeft === 0) return { key: 'page.cert.expiresToday', count: 0 }
  return { key: 'page.cert.daysLeft', count: daysLeft }
}

export const TLS_TONE_CLASS: Record<TlsTone, string> = {
  error: 'text-tone-danger',
  warn: 'text-tone-warning',
  ok: 'text-muted-foreground',
}
