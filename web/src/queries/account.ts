// Fetcher and hook for internal/api/account.go's handleChangePassword:
// PUT /api/v1/auth/password. Deliberately kept in its own file rather
// than folded into queries/auth.ts, whose own doc comment scopes itself
// explicitly to "internal/api/auth.go's three interactive routes" (login,
// register, logout). Change-password is a session-authenticated account
// action against a different handler file (account.go, not auth.go)
// entirely, so it gets its own module rather than stretching auth.ts's
// stated scope.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { sessionKeys } from './security'

export interface ChangePasswordCredentials {
  currentPassword: string
  newPassword: string
}

// 204 on success, no body to parse. 401 covers two distinct server-side
// cases collapsed into one status by design (handleChangePassword's own
// doc comment: no session at all, or a wrong current_password), and 400
// means new_password was shorter than the server's minPasswordLength.
// This fetcher does not try to guess which one happened, it just
// surfaces whatever message the server sent, the same "server response
// is authoritative" rule the account page's client-side Zod checks
// defer to.
export async function changePassword(
  credentials: ChangePasswordCredentials,
): Promise<void> {
  const res = await fetch('/api/v1/auth/password', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      current_password: credentials.currentPassword,
      new_password: credentials.newPassword,
    }),
  })
  if (!res.ok) {
    const message = await readErrorMessage(
      res,
      `change password failed: ${res.status}`,
    )
    throw new ApiError(res.status, message)
  }
}

export function useChangePassword() {
  return useMutation<void, ApiError, ChangePasswordCredentials>({
    mutationFn: changePassword,
  })
}

// PUT /api/v1/auth/email (internal/api/account.go's
// handleUpdateAdminEmail). 204 on success, no body. Pass "" to clear the
// recovery email back to unset.
export async function updateAdminEmail(email: string): Promise<void> {
  const res = await fetch('/api/v1/auth/email', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email }),
  })
  if (!res.ok) {
    const message = await readErrorMessage(
      res,
      `update email failed: ${res.status}`,
    )
    throw new ApiError(res.status, message)
  }
}

export function useUpdateAdminEmail() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: updateAdminEmail,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: sessionKeys.current() })
    },
  })
}
