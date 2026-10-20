// Abuse shield extras: internal/api/domain_policies_shield.go.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  base,
  domainPolicyKeys,
  jsonInit,
  request,
  type PolicyApiError,
} from './domainPolicies'
import type { ShieldBotList } from './domainPolicyTypes'

export const shieldKeys = {
  bots: ['system', 'shield', 'bots'] as const,
}

export function useShieldBots() {
  return useQuery({
    queryKey: shieldKeys.bots,
    queryFn: () =>
      request<ShieldBotList>(
        '/api/v1/system/shield/bots',
        undefined,
        'load bot list failed',
      ),
    staleTime: 300_000,
  })
}

export function useUnderAttack(app: string, domain: string) {
  const queryClient = useQueryClient()
  const url = `${base(app, domain)}/shield/under-attack`
  const onSuccess = () =>
    queryClient.invalidateQueries({
      queryKey: domainPolicyKeys.all(app, domain),
    })
  const start = useMutation<unknown, PolicyApiError, number>({
    mutationFn: (minutes) =>
      request(url, jsonInit('POST', { minutes }), 'start under attack failed'),
    onSuccess,
  })
  const stop = useMutation<unknown, PolicyApiError, void>({
    mutationFn: () => request(url, { method: 'DELETE' }, 'stand down failed'),
    onSuccess,
  })
  return { start, stop }
}
