import type { DrainNodeResponse } from '../types/nodeDetail'

// The backend repeats each blocked resource in errors ("service web: ..."
// or "model chat: ...") for older clients; the dialog lists those under
// Blocked, so failures are the errors that are not one of them.
export function drainFailures(result: DrainNodeResponse): string[] {
  const blockedPrefixes = (result.blocked ?? []).map(
    (b) => `${b.kind === 'app' ? 'service' : b.kind} ${b.name}: `,
  )
  return (result.errors ?? []).filter(
    (e) => !blockedPrefixes.some((p) => e.startsWith(p)),
  )
}
