// Fetchers and hooks for internal/api/password_reset.go's two
// unauthenticated routes: POST /api/v1/auth/forgot-password and POST
// /api/v1/auth/reset-password.

import { useMutation } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Always 204 by design (handleForgotPassword never reveals whether an
// admin or recovery email exists), so there is nothing to distinguish
// in a successful response.
export async function forgotPassword(): Promise<void> {
  const res = await fetch('/api/v1/auth/forgot-password', { method: 'POST' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `request failed: ${res.status}`),
    )
  }
}

export function useForgotPassword() {
  return useMutation<void, ApiError>({ mutationFn: forgotPassword })
}

export interface ResetPasswordArgs {
  token: string
  newPassword: string
}

export async function resetPassword({
  token,
  newPassword,
}: ResetPasswordArgs): Promise<void> {
  const res = await fetch('/api/v1/auth/reset-password', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ token, new_password: newPassword }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `reset failed: ${res.status}`),
    )
  }
}

export function useResetPassword() {
  return useMutation<void, ApiError, ResetPasswordArgs>({
    mutationFn: resetPassword,
  })
}
