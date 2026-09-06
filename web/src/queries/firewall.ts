// Query-key factory, fetcher, and sync mutation for
// GET/POST /api/v1/system/firewall(/sync) (internal/api/firewall.go):
// the per-port ufw rules this platform manages for exposed apps and
// databases. Session-cookie authenticated like every other settings
// query. Mirrors queries/systemDoctor.ts's read shape and
// queries/systemPrune.ts's "trigger, then invalidate the read query"
// mutation shape.

import { queryOptions, useMutation, useQueryClient, useSuspenseQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface FirewallRule {
  port: number
  proto: string
  owner: string
  open: boolean
}

export interface FirewallStatus {
  installed: boolean
  active: boolean
  rules: FirewallRule[]
  extra?: FirewallRule[]
}

export interface FirewallSyncResult extends FirewallStatus {
  applied: number
  removed: number
  errors?: string[]
}

export const firewallKeys = {
  all: ['system-firewall'] as const,
}

// 501 means no FirewallManager is configured on this control plane
// (WithFirewallManager), the same optional-dependency shape
// systemPrune.ts's own 501 branch handles for a missing Docker pruner.
export async function fetchFirewallStatus(): Promise<FirewallStatus> {
  const res = await fetch('/api/v1/system/firewall')
  if (res.status === 501) {
    throw new ApiError(501, 'Firewall management is not configured on this control plane.')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch firewall status failed: ${res.status}`),
    )
  }
  return (await res.json()) as FirewallStatus
}

export function firewallStatusQueryOptions() {
  return queryOptions({
    queryKey: firewallKeys.all,
    queryFn: fetchFirewallStatus,
  })
}

export function useFirewallStatus() {
  return useSuspenseQuery(firewallStatusQueryOptions())
}

export async function triggerFirewallSync(): Promise<FirewallSyncResult> {
  const res = await fetch('/api/v1/system/firewall/sync', { method: 'POST' })
  if (res.status === 501) {
    throw new ApiError(501, 'Firewall management is not configured on this control plane.')
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `firewall sync failed: ${res.status}`),
    )
  }
  return (await res.json()) as FirewallSyncResult
}

export function useTriggerFirewallSync() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: triggerFirewallSync,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: firewallKeys.all })
    },
  })
}
