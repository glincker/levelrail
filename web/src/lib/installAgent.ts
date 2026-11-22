import type { NodeResource } from '../types/nodeDetail'

/** True when the browser looks like it runs on a Mac, so the Mac tab opens first. */
export function looksLikeMac(userAgent: string): boolean {
  return /Macintosh|Mac OS X/i.test(userAgent)
}

/** The first node that was not there when the join command was created. */
export function findNewNode(
  nodes: readonly NodeResource[],
  baseline: readonly string[],
): NodeResource | undefined {
  return nodes.find((n) => !baseline.includes(n.id))
}
