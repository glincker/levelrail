import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { EnvApiError } from './envProtection'

export const ENCRYPTION_KEYS_STEP_UP_APP = 'encryption-keys'

export type KeyFixKind = 'rotate_master_key' | 'rotate_dek' | 'rotate_all_deks'

export interface EncryptionKeyHint {
  code: string
  severity: 'info' | 'warning'
  message: string
  fix: { kind: KeyFixKind; app?: string }
}

export interface AppKeyStatus {
  app: string
  created_at: string
  rotated_at?: string
  age_days: number
  value_count: number
  overdue: boolean
}

export interface KeyRotationRun {
  id: string
  started_at: string
  finished_at?: string
  total: number
  done: number
  failed: number
  actor?: string
}

// Matches internal/api's encryptionKeysResource.
export interface EncryptionKeysStatus {
  master_key: {
    configured: boolean
    rotated_at?: string
    age_days?: number
    overdue: boolean
  }
  apps: AppKeyStatus[]
  policy: {
    max_key_age_days: number
    remind_every_days: number
    last_reminded_at?: string
  }
  last_run?: KeyRotationRun
  hints: EncryptionKeyHint[]
}

export interface RotateDEKResult {
  app?: string
  values?: number
  rotated_at?: string
  run_id?: string
  resumed?: boolean
  total?: number
  rotated?: number
  skipped?: number
  failed_count?: number
  failed?: { app: string; reason: string }[]
  complete: boolean
}

export interface RotateDEKTarget {
  service?: string
  all?: boolean
}

export const encryptionKeyKeys = {
  status: ['encryption-keys'] as const,
}

async function failure(res: Response, fallback: string): Promise<never> {
  const body = (await res.json().catch(() => null)) as {
    error?: string
    code?: string
  } | null
  throw new EnvApiError(res.status, body?.error ?? fallback, body?.code ?? '')
}

export async function fetchEncryptionKeys(
  signal?: AbortSignal,
): Promise<EncryptionKeysStatus | null> {
  const res = await fetch('/api/v1/system/encryption-keys', {
    signal,
    cache: 'no-store',
  })
  if (res.status === 501) return null
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch encryption keys failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as EncryptionKeysStatus
}

export function useEncryptionKeys() {
  return useQuery({
    queryKey: encryptionKeyKeys.status,
    queryFn: ({ signal }) => fetchEncryptionKeys(signal),
    staleTime: 30_000,
  })
}

// Throws EnvApiError so useStepUpGate can catch step_up_required.
export async function rotateDEK(
  target: RotateDEKTarget,
): Promise<RotateDEKResult> {
  const res = await fetch('/api/v1/system/encryption-keys/rotate', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(target),
  })
  if (!res.ok) await failure(res, `rotate data key failed: ${res.status}`)
  return (await res.json()) as RotateDEKResult
}

export function useRotateDEK() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: rotateDEK,
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: encryptionKeyKeys.status }),
  })
}

export interface KeyPolicyUpdate {
  max_key_age_days?: number
  remind_every_days?: number
}

export function useUpdateKeyPolicy() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (
      update: KeyPolicyUpdate,
    ): Promise<EncryptionKeysStatus> => {
      const res = await fetch('/api/v1/system/encryption-keys/policy', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(update),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `update key policy failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as EncryptionKeysStatus
    },
    onSuccess: (status) =>
      queryClient.setQueryData(encryptionKeyKeys.status, status),
  })
}
