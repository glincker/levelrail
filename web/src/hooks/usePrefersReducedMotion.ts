import { useSyncExternalStore } from 'react'

const QUERY = '(prefers-reduced-motion: reduce)'

function subscribe(cb: () => void) {
  if (typeof window.matchMedia !== 'function') return () => undefined
  const mq = window.matchMedia(QUERY)
  mq.addEventListener('change', cb)
  return () => mq.removeEventListener('change', cb)
}

export function usePrefersReducedMotion(): boolean {
  return useSyncExternalStore(
    subscribe,
    () =>
      typeof window.matchMedia === 'function' &&
      window.matchMedia(QUERY).matches,
    () => false,
  )
}
