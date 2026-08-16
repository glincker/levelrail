// Fetchers and hooks for internal/api/account.go's session-info and
// revoke-other-sessions endpoints: GET /api/v1/auth/session and POST
// /api/v1/auth/sessions/revoke-others. Both are session-only like every
// other handler in that file (handleGetSession's own doc comment:
// "deliberately not requireAbility, a bearer token has no session of its
// own to report on"), so this module has no notion of "acting as a
// token" anywhere in it, same as queries/tokens.ts.

import {
  queryOptions,
  useMutation,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Matches internal/api/account.go's sessionInfoResponse. expires_at is
// RFC3339, formatted for display by the component, not here.
export interface SessionInfo {
  username: string
  display_name: string
  has_password: boolean
  expires_at: string
}

export const sessionKeys = {
  all: ['session'] as const,
  current: () => [...sessionKeys.all, 'current'] as const,
}

// GET /api/v1/auth/session (internal/api/account.go's handleGetSession).
export async function fetchSession(): Promise<SessionInfo> {
  const res = await fetch('/api/v1/auth/session')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch session failed: ${res.status}`),
    )
  }
  return (await res.json()) as SessionInfo
}

export function sessionQueryOptions() {
  return queryOptions({
    queryKey: sessionKeys.current(),
    queryFn: fetchSession,
  })
}

export function useSession() {
  return useSuspenseQuery(sessionQueryOptions())
}

// POST /api/v1/auth/sessions/revoke-others
// (internal/api/account.go's handleRevokeOtherSessions). Empty request
// body, 204 on success with no response body to parse. Revokes every
// OTHER live session for this admin account and leaves the session that
// made this request untouched, so there is nothing to invalidate on the
// session query itself: the caller's own username/expires_at are
// unchanged by this call.
export async function revokeOtherSessions(): Promise<void> {
  const res = await fetch('/api/v1/auth/sessions/revoke-others', {
    method: 'POST',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `revoke other sessions failed: ${res.status}`,
      ),
    )
  }
}

export function useRevokeOtherSessions() {
  return useMutation<void, ApiError>({
    mutationFn: revokeOtherSessions,
  })
}
