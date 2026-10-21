import { sidebarArea } from '../components/shell/sidebarArea'
import { appScopeName, databaseScopeName } from './sidebarScope'

export type TransitionType = 'forward' | 'back' | 'same'

// The slice of the router's location-change info this module reads.
interface LocationChange {
  fromLocation?: { pathname: string }
  toLocation: { pathname: string }
  pathChanged: boolean
}

// Identifies the thing a sidebar is about, so app A to app B counts as a move.
function scopeKey(pathname: string): string {
  const area = sidebarArea(pathname)
  if (area === 'app') return `app:${appScopeName(pathname) ?? ''}`
  if (area === 'database') return `db:${databaseScopeName(pathname) ?? ''}`
  return area
}

/**
 * Direction of a navigation: into a scoped area is forward, out to the main
 * nav is back, and a move inside one area is a quiet crossfade.
 */
export function transitionType(
  fromPath: string,
  toPath: string,
): TransitionType | null {
  if (fromPath === toPath) return null
  if (scopeKey(fromPath) === scopeKey(toPath)) return 'same'
  if (sidebarArea(fromPath) === 'global') return 'forward'
  if (sidebarArea(toPath) === 'global') return 'back'
  return 'forward'
}

/** Router option: false switches transitions off, the types drive the CSS in styles/routeTransitions.css. */
export function routeViewTransition():
  false | { types: (change: LocationChange) => string[] | false } {
  if (import.meta.env.VITE_ROUTE_TRANSITIONS === 'off') return false
  return {
    types: ({ fromLocation, toLocation, pathChanged }) => {
      if (!pathChanged || !fromLocation) return false
      if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
        return false
      }
      const type = transitionType(fromLocation.pathname, toLocation.pathname)
      return type ? [type] : false
    },
  }
}
