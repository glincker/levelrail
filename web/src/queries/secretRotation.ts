import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { appKeys } from './apps'

export type RotationStatus = 'ok' | 'due_soon' | 'overdue' | 'unknown'
export type RotationState =
  'idle' | 'restart_pending' | 'restart_failed' | 'failed'

export interface RotationPolicy {
  app?: string
  key: string
  interval_days: number
  grace_hours: number
  owner: string
  auto_rotate: boolean
  last_rotated_at?: string
  next_due_at?: string
  status: RotationStatus
  state: RotationState
  last_error?: string
  previous_expires_at?: string
}

export interface RotationPolicyInput {
  interval_days: number
  grace_hours: number
  owner: string
  auto_rotate: boolean
}

export const rotationKeys = {
  all: ['secret-rotation'] as const,
  app: (app: string) => [...rotationKeys.all, 'app', app] as const,
  overview: () => [...rotationKeys.all, 'overview'] as const,
}

const appBase = (app: string) => `/api/v1/apps/${encodeURIComponent(app)}`

async function ensureOk(res: Response, fallback: string): Promise<void> {
  if (!res.ok) {
    throw new ApiError(res.status, await readErrorMessage(res, fallback))
  }
}

export function useAppRotationPolicies(app: string) {
  return useQuery({
    queryKey: rotationKeys.app(app),
    queryFn: async (): Promise<RotationPolicy[]> => {
      const res = await fetch(`${appBase(app)}/secret-rotation`)
      if (res.status === 501) return []
      await ensureOk(
        res,
        `list rotation policies failed: ${String(res.status)}`,
      )
      return (await res.json()) as RotationPolicy[]
    },
  })
}

export function useRotationOverview() {
  return useQuery({
    queryKey: rotationKeys.overview(),
    queryFn: async (): Promise<RotationPolicy[]> => {
      const res = await fetch('/api/v1/secret-rotation')
      if (res.status === 501 || res.status === 403) return []
      await ensureOk(
        res,
        `list rotation policies failed: ${String(res.status)}`,
      )
      return (await res.json()) as RotationPolicy[]
    },
  })
}

function useInvalidate(app: string) {
  const queryClient = useQueryClient()
  return () => {
    void queryClient.invalidateQueries({ queryKey: rotationKeys.all })
    void queryClient.invalidateQueries({ queryKey: appKeys.detail(app) })
  }
}

export function useSetRotationPolicy(app: string) {
  const invalidate = useInvalidate(app)
  return useMutation({
    mutationFn: async (input: RotationPolicyInput & { key: string }) => {
      const { key, ...body } = input
      const res = await fetch(
        `${appBase(app)}/secrets/${encodeURIComponent(key)}/rotation`,
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        },
      )
      await ensureOk(res, `save rotation policy failed: ${String(res.status)}`)
    },
    onSuccess: invalidate,
  })
}

export function useDeleteRotationPolicy(app: string) {
  const invalidate = useInvalidate(app)
  return useMutation({
    mutationFn: async (key: string) => {
      const res = await fetch(
        `${appBase(app)}/secrets/${encodeURIComponent(key)}/rotation`,
        { method: 'DELETE' },
      )
      await ensureOk(
        res,
        `remove rotation policy failed: ${String(res.status)}`,
      )
    },
    onSuccess: invalidate,
  })
}

export function useRotateSecretValue(app: string) {
  const invalidate = useInvalidate(app)
  return useMutation({
    mutationFn: async (input: { key: string; value: string }) => {
      const res = await fetch(
        `${appBase(app)}/secrets/${encodeURIComponent(input.key)}/rotate`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(input.value ? { value: input.value } : {}),
        },
      )
      await ensureOk(res, `rotate secret failed: ${String(res.status)}`)
    },
    onSettled: invalidate,
  })
}

export function useRotateNow(app: string) {
  const invalidate = useInvalidate(app)
  return useMutation({
    mutationFn: async (key: string) => {
      const res = await fetch(
        `${appBase(app)}/secrets/${encodeURIComponent(key)}/rotation/rotate`,
        { method: 'POST' },
      )
      await ensureOk(res, `rotate secret failed: ${String(res.status)}`)
    },
    onSettled: invalidate,
  })
}
