import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { EnvApiError } from './envProtection'
import { databaseKeys } from './databases'

export const DB_PASSWORD_STEP_UP_APP = 'database-password'

// Matches internal/api's databasePasswordRotationResource.
export interface DatabasePasswordRotation {
  database: string
  engine: string
  supported: boolean
  reason?: string
  restart_required: boolean
  apps: string[]
  suspended_apps: string[]
  state: 'idle' | 'pending' | 'failed'
  last_error?: string
  last_changed_at?: string
  age_days?: number
  interval_days: number
  next_due?: string
  restarted_apps?: string[]
  rotated_at?: string
}

export const databasePasswordRotationKeys = {
  detail: (name: string) =>
    [...databaseKeys.detail(name), 'password-rotation'] as const,
}

const path = (name: string) =>
  `/api/v1/databases/${encodeURIComponent(name)}/password-rotation`

async function failure(res: Response, fallback: string): Promise<never> {
  const body = (await res.json().catch(() => null)) as {
    error?: string
    code?: string
  } | null
  throw new EnvApiError(res.status, body?.error ?? fallback, body?.code ?? '')
}

export function stepUpScopeFor(name: string) {
  return {
    action: 'manage',
    app: `${DB_PASSWORD_STEP_UP_APP}:${name}`,
  } as const
}

export function useDatabasePasswordRotation(name: string) {
  return useQuery({
    queryKey: databasePasswordRotationKeys.detail(name),
    queryFn: async ({ signal }): Promise<DatabasePasswordRotation | null> => {
      const res = await fetch(path(name), { signal, cache: 'no-store' })
      if (res.status === 501 || res.status === 403) return null
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(
            res,
            `fetch password rotation failed: ${res.status}`,
          ),
        )
      }
      return (await res.json()) as DatabasePasswordRotation
    },
    staleTime: 15_000,
  })
}

export async function previewDatabasePasswordRotation(
  name: string,
): Promise<DatabasePasswordRotation> {
  const res = await fetch(path(name), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ dry_run: true }),
  })
  if (!res.ok) await failure(res, `preview failed: ${res.status}`)
  return (await res.json()) as DatabasePasswordRotation
}

// Throws EnvApiError so useStepUpGate can catch step_up_required.
export async function rotateDatabasePassword(
  name: string,
): Promise<DatabasePasswordRotation> {
  const res = await fetch(path(name), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: '{}',
  })
  if (!res.ok) await failure(res, `rotate password failed: ${res.status}`)
  return (await res.json()) as DatabasePasswordRotation
}

export function useRotateDatabasePassword(name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => rotateDatabasePassword(name),
    onSettled: () =>
      queryClient.invalidateQueries({
        queryKey: databasePasswordRotationKeys.detail(name),
      }),
  })
}

export function useSetDatabasePasswordInterval(name: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (days: number): Promise<DatabasePasswordRotation> => {
      const res = await fetch(`${path(name)}/policy`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ interval_days: days }),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `save schedule failed: ${res.status}`),
        )
      }
      return (await res.json()) as DatabasePasswordRotation
    },
    onSuccess: (data) =>
      queryClient.setQueryData(databasePasswordRotationKeys.detail(name), data),
  })
}
