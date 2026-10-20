import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Matches internal/api's authEngineStatusResponse.
export interface AuthEngineStatus {
  library_version: string
  totp: boolean
  passkeys: boolean
  oauth: boolean
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
  })
}

export function useAuthEngineStatus() {
  return useQuery(authEngineStatusQueryOptions())
}
