// Last-used node per user and form, so a repeat deploy starts where the
// previous one went. Storage can throw (private mode), so every call is guarded.
const PREFIX = 'levelrail:last-deploy-target'

function keyFor(user: string | null, scope: string): string {
  return `${PREFIX}:${user ?? 'anon'}:${scope}`
}

export function readLastDeployTarget(
  user: string | null,
  scope: string,
): string | null {
  try {
    return window.localStorage.getItem(keyFor(user, scope))
  } catch {
    return null
  }
}

export function writeLastDeployTarget(
  user: string | null,
  scope: string,
  nodeId: string,
): void {
  try {
    window.localStorage.setItem(keyFor(user, scope), nodeId)
  } catch {
    // Remembering the choice is a convenience only.
  }
}
