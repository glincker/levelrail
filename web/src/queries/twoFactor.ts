// Fetchers and hooks for internal/api/twofactor.go's account-scoped 2FA
// routes: status, setup, confirm, disable, and recovery-code
// regeneration. All session-only (requireAuth on the backend), same
// shape queries/security.ts already uses for this account's own auth
// state. The login-time verify step (POST /api/v1/auth/2fa/verify) is
// deliberately not here: it's part of the sign-in flow, not an
// account-settings action, and lives in queries/auth.ts next to login
// itself.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type {
  TwoFactorRecoveryCodes,
  TwoFactorSetup,
  TwoFactorStatus,
} from '../types/twoFactor'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const twoFactorKeys = {
  all: ['two-factor'] as const,
  status: () => [...twoFactorKeys.all, 'status'] as const,
}

async function throwTwoFactorError(
  res: Response,
  fallback: string,
): Promise<never> {
  throw new ApiError(res.status, await readErrorMessage(res, fallback))
}

// GET /api/v1/auth/2fa (handleGetTwoFactorStatus).
export async function fetchTwoFactorStatus(): Promise<TwoFactorStatus> {
  const res = await fetch('/api/v1/auth/2fa')
  if (!res.ok) {
    await throwTwoFactorError(
      res,
      `fetch two-factor status failed: ${res.status}`,
    )
  }
  return (await res.json()) as TwoFactorStatus
}

export function twoFactorStatusQueryOptions() {
  return queryOptions({
    queryKey: twoFactorKeys.status(),
    queryFn: fetchTwoFactorStatus,
  })
}

export function useTwoFactorStatus() {
  return useSuspenseQuery(twoFactorStatusQueryOptions())
}

// POST /api/v1/auth/2fa/setup (handleSetupTwoFactor): mints a fresh,
// unconfirmed secret. Does not invalidate the status query itself,
// enabled only flips true once handleConfirmTwoFactor succeeds.
export async function setupTwoFactor(): Promise<TwoFactorSetup> {
  const res = await fetch('/api/v1/auth/2fa/setup', { method: 'POST' })
  if (!res.ok) {
    await throwTwoFactorError(res, `setup two-factor failed: ${res.status}`)
  }
  return (await res.json()) as TwoFactorSetup
}

export function useSetupTwoFactor() {
  return useMutation<TwoFactorSetup, ApiError, void>({
    mutationFn: setupTwoFactor,
  })
}

// POST /api/v1/auth/2fa/confirm (handleConfirmTwoFactor).
export async function confirmTwoFactor(
  code: string,
): Promise<TwoFactorRecoveryCodes> {
  const res = await fetch('/api/v1/auth/2fa/confirm', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code }),
  })
  if (!res.ok) {
    await throwTwoFactorError(res, `confirm two-factor failed: ${res.status}`)
  }
  return (await res.json()) as TwoFactorRecoveryCodes
}

export function useConfirmTwoFactor() {
  const queryClient = useQueryClient()
  return useMutation<TwoFactorRecoveryCodes, ApiError, string>({
    mutationFn: confirmTwoFactor,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: twoFactorKeys.all })
    },
  })
}

export interface DisableTwoFactorRequest {
  code?: string
  recoveryCode?: string
}

// POST /api/v1/auth/2fa/disable (handleDisableTwoFactor). 204 on
// success, no body to parse.
export async function disableTwoFactor(
  req: DisableTwoFactorRequest,
): Promise<void> {
  const res = await fetch('/api/v1/auth/2fa/disable', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      code: req.code ?? '',
      recovery_code: req.recoveryCode ?? '',
    }),
  })
  if (res.status === 204) {
    return
  }
  await throwTwoFactorError(res, `disable two-factor failed: ${res.status}`)
}

export function useDisableTwoFactor() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, DisableTwoFactorRequest>({
    mutationFn: disableTwoFactor,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: twoFactorKeys.all })
    },
  })
}

// POST /api/v1/auth/2fa/recovery-codes/regenerate
// (handleRegenerateRecoveryCodes).
export async function regenerateRecoveryCodes(
  code: string,
): Promise<TwoFactorRecoveryCodes> {
  const res = await fetch('/api/v1/auth/2fa/recovery-codes/regenerate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code }),
  })
  if (!res.ok) {
    await throwTwoFactorError(
      res,
      `regenerate recovery codes failed: ${res.status}`,
    )
  }
  return (await res.json()) as TwoFactorRecoveryCodes
}

export function useRegenerateRecoveryCodes() {
  const queryClient = useQueryClient()
  return useMutation<TwoFactorRecoveryCodes, ApiError, string>({
    mutationFn: regenerateRecoveryCodes,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: twoFactorKeys.all })
    },
  })
}
