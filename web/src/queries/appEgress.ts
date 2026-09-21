// Query-key factory and fetchers for GET/PUT/DELETE
// /api/v1/apps/{name}/egress-policy (internal/api/apps_egress.go): an
// app's outbound network allowlist, enforced by a reconciled egress
// sidecar (internal/reconcile/application/egress.go). Unconfigured
// (mode/allow both absent) means unrestricted egress, today's default
// for every app. Mirrors queries/domainRedirect.ts's shape: GET/PUT
// return the full resource, DELETE clears it back to unconfigured.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { deployKeys } from './deploys'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Mirrors internal/api's egressAllowResource wire shape exactly.
export interface EgressAllowRule {
  host: string
  port: number
}

// Mirrors internal/api's egressPolicyResource wire shape exactly: mode
// and allow are both absent (not empty/zero-valued) when the app has no
// egress policy configured.
export interface EgressPolicy {
  app_name: string
  mode?: string
  allow?: EgressAllowRule[]
}

// Mirrors internal/api's setEgressPolicyRequest wire shape exactly. The
// only meaningful mode value today is "allowlist"
// (store.EgressModeAllowlist).
export interface SetEgressPolicyRequest {
  mode: string
  allow: EgressAllowRule[]
}

export const appEgressKeys = {
  detail: (appName: string) =>
    [...appKeys.detail(appName), 'egress-policy'] as const,
}

function egressPolicyPath(appName: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/egress-policy`
}

export async function fetchAppEgressPolicy(
  appName: string,
): Promise<EgressPolicy> {
  const res = await fetch(egressPolicyPath(appName))
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch egress policy failed: ${res.status}`),
    )
  }
  return (await res.json()) as EgressPolicy
}

export function appEgressPolicyQueryOptions(appName: string) {
  return queryOptions({
    queryKey: appEgressKeys.detail(appName),
    queryFn: () => fetchAppEgressPolicy(appName),
  })
}

export function useAppEgressPolicy(appName: string) {
  return useQuery(appEgressPolicyQueryOptions(appName))
}

async function setAppEgressPolicy(
  appName: string,
  req: SetEgressPolicyRequest,
): Promise<EgressPolicy> {
  const res = await fetch(egressPolicyPath(appName), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `set egress policy failed: ${res.status}`),
    )
  }
  return (await res.json()) as EgressPolicy
}

export function useSetAppEgressPolicy(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<EgressPolicy, ApiError, SetEgressPolicyRequest>({
    mutationFn: (req) => setAppEgressPolicy(appName, req),
    onSuccess: (updated) => {
      queryClient.setQueryData(appEgressKeys.detail(appName), updated)
      // A freshly saved policy has to actually be applied by the
      // reconciler before EgressPolicyReady reflects it; invalidate the
      // reconciler-side conditions query so the live status badge
      // doesn't keep showing a stale pre-save reason.
      void queryClient.invalidateQueries({
        queryKey: deployKeys.status(appName),
      })
    },
  })
}

// Response is 204 No Content (internal/api/apps_egress.go's
// handleClearAppEgressPolicy), mirroring queries/apps.ts's own
// clearAppStorage shape for the same reason: nothing meaningful to
// parse as JSON on a successful clear.
async function clearAppEgressPolicy(appName: string): Promise<void> {
  const res = await fetch(egressPolicyPath(appName), { method: 'DELETE' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `clear egress policy failed: ${res.status}`),
    )
  }
}

export function useClearAppEgressPolicy(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: () => clearAppEgressPolicy(appName),
    onSuccess: () => {
      queryClient.setQueryData(appEgressKeys.detail(appName), {
        app_name: appName,
      } satisfies EgressPolicy)
      void queryClient.invalidateQueries({
        queryKey: deployKeys.status(appName),
      })
    },
  })
}
