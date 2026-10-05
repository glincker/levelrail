import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface AuthEngineMismatch {
  at: string
  kind: string
  token_id?: string
  legacy_owner_id?: string
  library_owner_id?: string
  legacy_accepted: boolean
  library_accepted: boolean
  legacy_abilities: string[]
  library_abilities: string[]
}

// Matches internal/api's authEngineStatusResponse.
export interface AuthEngineStatus {
  mode: 'legacy' | 'shadow' | 'library'
  library_version: string
  areas: string[]
  compared: number
  matched: number
  mismatched: number
  dropped: number
  skipped: number
  errors: number
  mismatches: AuthEngineMismatch[]
}

export const authEngineKeys = {
  all: ['auth-engine'] as const,
  status: () => [...authEngineKeys.all, 'status'] as const,
}

// GET /api/v1/auth-engine/status. Root only: a 403 means "not for this user", not an error to show.
async function fetchAuthEngineStatus(): Promise<AuthEngineStatus | null> {
  const res = await fetch('/api/v1/auth-engine/status')
  if (res.status === 403) return null
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch auth engine status failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as AuthEngineStatus
}

export function authEngineStatusQueryOptions() {
  return queryOptions({
    queryKey: authEngineKeys.status(),
    queryFn: fetchAuthEngineStatus,
    refetchInterval: 15_000,
  })
}

export function useAuthEngineStatus() {
  return useQuery(authEngineStatusQueryOptions())
}
