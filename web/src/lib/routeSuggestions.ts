import { closestNames } from '@/lib/closestNames'
import { NAV_REGISTRY } from '@/lib/navRegistry'

const SUGGESTION_LIMIT = 3

function lastSegment(path: string): string {
  const parts = path.split('/').filter(Boolean)
  return parts[parts.length - 1] ?? ''
}

/** suggestPages lists registered pages whose last path segment matches or is close to the requested one. */
export function suggestPages(pathname: string) {
  const tail = lastSegment(pathname).toLowerCase()
  if (!tail) return []
  const pages = NAV_REGISTRY.filter((e) => e.path !== '/')
  // Old or guessed URLs (/traffic, /registry) usually name a page one level deeper.
  const exact = pages.filter((e) => lastSegment(e.path) === tail)
  const near = closestNames(
    tail,
    [...new Set(pages.map((e) => lastSegment(e.path)))],
    SUGGESTION_LIMIT,
  )
  const nearPages = near.flatMap((seg) =>
    pages.filter((e) => lastSegment(e.path) === seg),
  )
  const seen = new Set<string>()
  return [...exact, ...nearPages]
    .filter((e) => (seen.has(e.path) ? false : (seen.add(e.path), true)))
    .slice(0, SUGGESTION_LIMIT)
}
