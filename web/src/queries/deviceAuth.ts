// Query-key factory and fetchers for /api/v1/auth/device
// (internal/api/device_auth.go): the web half of "levelrail-cli auth
// login --device". The CLI's own start/token endpoints are
// unauthenticated (there is no credential yet), but requests, approve,
// and deny are all session-only (requireAuth, routes.go), matching this
// codebase's existing fetch convention of relying on the browser's
// cookie rather than any bearer header.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Mirrors device_auth.go's deviceAuthRequestResource exactly. This list
// only ever contains pending, not-yet-expired requests: the server
// filters, this file has no notion of a decided or expired request.
export interface DeviceAuthRequest {
  user_code: string
  client_name: string
  created_at: string
  expires_at: string
}

export const deviceAuthKeys = {
  all: ['device-auth-requests'] as const,
  list: () => [...deviceAuthKeys.all, 'list'] as const,
}

export async function fetchDeviceAuthRequests(): Promise<DeviceAuthRequest[]> {
  const res = await fetch('/api/v1/auth/device/requests')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch device login requests failed: ${res.status}`),
    )
  }
  return (await res.json()) as DeviceAuthRequest[]
}

export function deviceAuthRequestsQueryOptions() {
  return queryOptions({
    queryKey: deviceAuthKeys.list(),
    queryFn: fetchDeviceAuthRequests,
  })
}

// A pending request stays valid for 10 minutes (deviceAuthTTL,
// device_auth.go) and the operator is actively watching this page for a
// code to show up, so a short fixed poll is the deliberate choice here,
// the same pattern queries/appNetwork.ts uses for other live state.
const DEVICE_AUTH_POLL_INTERVAL_MS = 4_000

export function useDeviceAuthRequests() {
  return useSuspenseQuery({
    ...deviceAuthRequestsQueryOptions(),
    refetchInterval: DEVICE_AUTH_POLL_INTERVAL_MS,
  })
}

async function decideDeviceAuthRequest(
  userCode: string,
  action: 'approve' | 'deny',
): Promise<void> {
  const res = await fetch(
    `/api/v1/auth/device/${encodeURIComponent(userCode)}/${action}`,
    { method: 'POST' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${action} device login failed: ${res.status}`),
    )
  }
}

export function useApproveDeviceAuthRequest() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (userCode: string) =>
      decideDeviceAuthRequest(userCode, 'approve'),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: deviceAuthKeys.list() })
    },
  })
}

export function useDenyDeviceAuthRequest() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (userCode: string) =>
      decideDeviceAuthRequest(userCode, 'deny'),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: deviceAuthKeys.list() })
    },
  })
}
