// Shared HTTP-error helpers for every fetcher under web/src/queries/*.ts.
// Centralizing this here means a 401 (no session, or one that expired
// mid-use) can be detected in exactly one place (isUnauthorized) instead
// of every fetcher re-implementing its own status check, and the app
// shell's global QueryCache/MutationCache handler (see main.tsx) can react
// to it uniformly: redirect to /login, distinct from how a 404/500 still
// renders in each route's own error boundary. Closes
// docs-local/research/dashboard-gap-audit-and-devmode.md gap #2.

export class ApiError extends Error {
  readonly status: number

  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

export function isUnauthorized(error: unknown): boolean {
  return error instanceof ApiError && error.status === 401
}

// Reads the `{"error": "..."}` body shape every internal/api handler's
// writeError produces, falling back to a generic message if the body
// isn't JSON or has no `error` field (e.g. a proxy-generated error page
// in front of the control plane).
export async function readErrorMessage(
  res: Response,
  fallback: string,
): Promise<string> {
  const body = (await res.json().catch(() => null)) as { error?: string } | null
  return body?.error ?? fallback
}
