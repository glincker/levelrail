import { useCallback } from 'react'
import { useRouter } from '@tanstack/react-router'

/** True when `to` is one of the registered route paths (with or without a trailing slash). */
export function hasRoute(registered: readonly string[], to: string): boolean {
  const bare = to.length > 1 ? to.replace(/\/$/, '') : to
  return registered.some(
    (path) => path === bare || path === `${bare}/` || path === to,
  )
}

/**
 * Predicate for "does the router know this path yet". Lets nav, shortcuts and
 * the palette hide entries whose pages land in a later change instead of
 * sending people to a 404.
 */
export function useRouteAvailable(): (to: string) => boolean {
  const router = useRouter()
  return useCallback(
    (to: string) => hasRoute(Object.keys(router.routesByPath), to),
    [router],
  )
}
