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
  useQuery,
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
  requester_ip: string
  user_agent: string
  // True when the requesting IP differs from this session's own IP.
  ip_mismatch: boolean
}

export type DeviceLoginState = 'waiting' | 'expired' | 'denied' | 'approved'

// Mirrors device_activity.go's deviceActivityItem. Never carries a code.
export interface DeviceActivityItem {
  id: string
  state: DeviceLoginState
  client_name: string
  requester_ip: string
  user_agent: string
  created_at: string
  expires_at: string
  ip_mismatch: boolean
  dismissible: boolean
  dismissed: boolean
  item_key: string
  audit_path: string
}

export const deviceAuthKeys = {
  all: ['device-auth-requests'] as const,
  list: () => [...deviceAuthKeys.all, 'list'] as const,
  activity: () => [...deviceAuthKeys.all, 'activity'] as const,
}

export async function fetchDeviceActivity(): Promise<DeviceActivityItem[]> {
  const res = await fetch('/api/v1/auth/device/activity')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch device login activity failed: ${res.status}`,
      ),
    )
  }
  const body = (await res.json()) as { items?: DeviceActivityItem[] }
  return Array.isArray(body.items) ? body.items : []
}

export function deviceActivityQueryOptions() {
  return queryOptions({
    queryKey: deviceAuthKeys.activity(),
    queryFn: fetchDeviceActivity,
  })
}

export async function fetchDeviceAuthRequests(): Promise<DeviceAuthRequest[]> {
  const res = await fetch('/api/v1/auth/device/requests')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch device login requests failed: ${res.status}`,
      ),
    )
  }
  const body: unknown = await res.json()
  return Array.isArray(body) ? (body as DeviceAuthRequest[]) : []
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

// Shell-wide live view for the attention bell and banner. A failing
// endpoint (signed out, token-only session) just yields no requests, and
// TanStack Query pauses interval refetching while the tab is hidden.
export function useLiveDeviceAuthRequests() {
  const activity = useDeviceActivity()
  const waiting = activity.data?.filter((i) => i.state === 'waiting').length
  const query = useQuery({
    ...deviceAuthRequestsQueryOptions(),
    retry: false,
    enabled: waiting !== 0,
    refetchInterval: DEVICE_AUTH_POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  })
  return waiting === 0 ? { ...query, data: [] } : query
}

// The one device-login poll in steady state: every state of a recent
// login, so expired and denied items show without another source. The
// code-bearing list above is fetched only while something is waiting.
export function useDeviceActivity() {
  return useQuery({
    ...deviceActivityQueryOptions(),
    retry: false,
    refetchInterval: DEVICE_AUTH_POLL_INTERVAL_MS,
    refetchIntervalInBackground: false,
  })
}

export function useDismissAttentionItem() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (itemKey: string): Promise<void> => {
      const res = await fetch('/api/v1/attention/dismiss', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ item_key: itemKey }),
      })
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `dismiss failed: ${res.status}`),
        )
      }
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: deviceAuthKeys.activity(),
      })
    },
  })
}

export function isDeviceRequestLive(
  request: DeviceAuthRequest,
  now: number,
): boolean {
  return new Date(request.expires_at).getTime() > now
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
      await readErrorMessage(
        res,
        `${action} device login failed: ${res.status}`,
      ),
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
    mutationFn: (userCode: string) => decideDeviceAuthRequest(userCode, 'deny'),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: deviceAuthKeys.list() })
    },
  })
}
