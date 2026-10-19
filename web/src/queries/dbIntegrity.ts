import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { EnvApiError } from './envProtection'

export interface IntegrityStatus {
  healthy: boolean
  mode: 'quick' | 'full'
  checked_at: string
  error?: string
  checks: number
  failures: number
  last_full_at: string
}

export interface IntegrityPending {
  name: string
  sha256: string
  schema_version: number
  staged_at: string
  staged_by?: string
}

export interface IntegrityCandidate {
  name: string
  size_bytes: number
  created_at: string
  verified_at?: string
  verified: boolean
}

export interface IntegrityView {
  status: IntegrityStatus
  pending?: IntegrityPending
  candidates: IntegrityCandidate[]
}

const URL = '/api/v1/system/integrity'
const KEY = ['system-integrity']

async function failure(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export function useIntegrity() {
  return useQuery({
    queryKey: KEY,
    queryFn: async ({ signal }): Promise<IntegrityView> => {
      const res = await fetch(URL, { signal, cache: 'no-store' })
      if (!res.ok) throw await failure(res, 'fetch database integrity')
      return (await res.json()) as IntegrityView
    },
    refetchInterval: 30_000,
  })
}

export function useCheckIntegrity() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (full: boolean): Promise<IntegrityStatus> => {
      const res = await fetch(`${URL}/check`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ full }),
      })
      if (!res.ok) throw await failure(res, 'integrity check')
      return (await res.json()) as IntegrityStatus
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: KEY })
      void qc.invalidateQueries({ queryKey: ['control-plane-health'] })
    },
  })
}

export function useStageRestore() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (name: string): Promise<IntegrityPending> => {
      const res = await fetch(`${URL}/restore`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name }),
      })
      if (!res.ok) {
        const body = (await res
          .clone()
          .json()
          .catch(() => null)) as { error?: string; code?: string } | null
        if (body?.code) {
          throw new EnvApiError(
            res.status,
            body.error ?? `stage restore failed: ${res.status}`,
            body.code,
          )
        }
        throw await failure(res, 'stage restore')
      }
      const body = (await res.json()) as { pending: IntegrityPending }
      return body.pending
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: KEY })
    },
  })
}

export function useCancelRestore() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (): Promise<void> => {
      const res = await fetch(`${URL}/restore`, { method: 'DELETE' })
      if (!res.ok) throw await failure(res, 'cancel restore')
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: KEY })
    },
  })
}
