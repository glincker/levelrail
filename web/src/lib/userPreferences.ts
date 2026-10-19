import { useSyncExternalStore } from 'react'

let timezone = ''
const listeners = new Set<() => void>()

function validZone(tz: string): boolean {
  if (tz === '') return true
  try {
    new Intl.DateTimeFormat(undefined, { timeZone: tz })
    return true
  } catch {
    return false
  }
}

// An empty or invalid zone means "the browser's own".
export function setUserTimezone(tz: string): void {
  const next = validZone(tz) ? tz : ''
  if (next === timezone) return
  timezone = next
  listeners.forEach((l) => l())
}

export function getUserTimezone(): string {
  return timezone
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

export function useUserTimezone(): string {
  return useSyncExternalStore(subscribe, getUserTimezone, () => '')
}

export function formatUserDateTime(
  iso: string | undefined,
  fallback = '',
  zone: string = timezone,
): string {
  if (!iso) return fallback
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return fallback
  return date.toLocaleString(
    undefined,
    zone === '' ? undefined : { timeZone: zone },
  )
}

export function browserTimezone(): string {
  return Intl.DateTimeFormat().resolvedOptions().timeZone
}

export function supportedTimezones(): string[] {
  const fn = (
    Intl as unknown as { supportedValuesOf?: (k: string) => string[] }
  ).supportedValuesOf
  return typeof fn === 'function' ? fn('timeZone') : [browserTimezone()]
}
