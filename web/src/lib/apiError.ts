// Shared HTTP-error helpers for every fetcher under web/src/queries/*.ts.
// Centralizing this here means a 401 (no session, or one that expired
// mid-use) can be detected in exactly one place (isUnauthorized) instead
// of every fetcher re-implementing its own status check, and the app
// shell's global QueryCache/MutationCache handler (see main.tsx) can react
// to it uniformly: redirect to /login, distinct from how a 404/500 still
// renders in each route's own error boundary.

export interface ApiErrorFix {
  id: string
  label: string
}

interface TypedDetails {
  code: string
  fix?: ApiErrorFix
  requestId?: string
}

// readErrorMessage records typed bodies here, keyed by their message, so the
// ApiError every fetcher builds from that message carries code, fix and request id.
const typedDetails = new Map<string, TypedDetails>()
const MAX_TYPED_DETAILS = 50

export class ApiError extends Error {
  readonly status: number
  readonly code?: string
  readonly fix?: ApiErrorFix
  readonly requestId?: string

  constructor(status: number, message: string, details?: TypedDetails) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    const d = details ?? typedDetails.get(message)
    if (d) {
      this.code = d.code
      this.fix = d.fix
      this.requestId = d.requestId
    }
  }
}

export function isUnauthorized(error: unknown): boolean {
  return error instanceof ApiError && error.status === 401
}

// TanStack Router's own errorComponent props type error as unknown, not
// Error, since a thrown value is never actually guaranteed to be one.
export function routeErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error)
}

// Reads the `{"error": "..."}` body shape every internal/api handler's
// writeError produces, falling back to a generic message if the body
// isn't JSON or has no `error` field (e.g. a proxy-generated error page
// in front of the control plane).
export async function readErrorMessage(
  res: Response,
  fallback: string,
): Promise<string> {
  const body = (await res.json().catch(() => null)) as {
    error?: string
    message?: string
    code?: string
    fix?: ApiErrorFix
    request_id?: string
  } | null
  if (body?.code && body.message) {
    if (typedDetails.size >= MAX_TYPED_DETAILS) typedDetails.clear()
    typedDetails.set(body.message, {
      code: body.code,
      fix: body.fix,
      requestId: body.request_id,
    })
    return body.message
  }
  // Demo gate bodies carry a machine code in `error` and the human text in `message`.
  if (body?.error?.startsWith('demo_') && body.message) {
    return body.message
  }
  return body?.error ?? fallback
}

export interface ExistingVolumeInfo {
  name: string
  size_bytes: number
  mounted: boolean
}

// Thrown by createDatabase when a deleted database of the same name left its
// data volume behind, so the form can ask reuse or discard instead of failing.
export class ExistingVolumeError extends ApiError {
  readonly volumes: ExistingVolumeInfo[]

  constructor(message: string, volumes: ExistingVolumeInfo[]) {
    super(409, message)
    this.name = 'ExistingVolumeError'
    this.volumes = volumes
  }
}
