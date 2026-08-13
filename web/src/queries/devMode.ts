// Query-key factory and fetcher for GET /api/v1/dev-mode
// (internal/api/devmode.go's handleDevMode), the public endpoint the
// login screen reads to decide whether to offer a one-click dev login
// button. Public and safe to call before a session exists, same shape
// as brand.ts's fetchBrand.

import { queryOptions } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const devModeKeys = {
  all: ['dev-mode'] as const,
}

interface DevModeStatus {
  enabled: boolean
}

export async function fetchDevMode(): Promise<DevModeStatus> {
  const res = await fetch('/api/v1/dev-mode')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch dev mode status failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as DevModeStatus
}

// staleTime: Infinity, same reasoning as brandQueryOptions: whether
// APP_DEV_MODE is set is fixed for the lifetime of a running control
// plane process, never toggled mid-session.
export function devModeQueryOptions() {
  return queryOptions({
    queryKey: devModeKeys.all,
    queryFn: fetchDevMode,
    staleTime: Infinity,
  })
}
