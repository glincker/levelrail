// Query-key factory and fetchers for /api/v1/firewall-rules, mirroring
// queries/notificationChannels.ts's own shared-queryOptions pattern.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type {
  CreateFirewallRuleRequest,
  FirewallRule,
} from '../types/firewallRule'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const firewallRuleKeys = {
  all: ['firewall-rules'] as const,
  list: () => [...firewallRuleKeys.all, 'list'] as const,
}

export async function fetchFirewallRules(): Promise<FirewallRule[]> {
  const res = await fetch('/api/v1/firewall-rules')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch firewall rules failed: ${res.status}`),
    )
  }
  return (await res.json()) as FirewallRule[]
}

export function firewallRuleListQueryOptions() {
  return queryOptions({
    queryKey: firewallRuleKeys.list(),
    queryFn: fetchFirewallRules,
  })
}

export function useFirewallRules() {
  return useSuspenseQuery(firewallRuleListQueryOptions())
}

export async function createFirewallRule(
  req: CreateFirewallRuleRequest,
): Promise<FirewallRule> {
  const res = await fetch('/api/v1/firewall-rules', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create firewall rule failed: ${res.status}`),
    )
  }
  return (await res.json()) as FirewallRule
}

export function useCreateFirewallRule() {
  const queryClient = useQueryClient()
  return useMutation<FirewallRule, ApiError, CreateFirewallRuleRequest>({
    mutationFn: createFirewallRule,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: firewallRuleKeys.list() })
    },
  })
}

export async function deleteFirewallRule(id: string): Promise<void> {
  const res = await fetch(`/api/v1/firewall-rules/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(res, `delete firewall rule failed: ${res.status}`),
  )
}

export function useDeleteFirewallRule() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: deleteFirewallRule,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: firewallRuleKeys.list() })
    },
  })
}
