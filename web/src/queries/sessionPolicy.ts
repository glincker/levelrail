// Fetchers and hooks for internal/api/access_policy_handlers.go: per-team
// session policies (root only) and the caller's effective policy.

import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import type { ApiError } from '../lib/apiError'
import { ssoRequest } from './sso'

export type SessionOnLimit = 'evict_oldest' | 'reject'

export interface SessionPolicy {
  team_id: string
  team_name: string
  idle_timeout: string
  max_lifetime: string
  ip_allowlist: string[]
  require_mfa: boolean
  max_concurrent_sessions: number
  on_limit: SessionOnLimit
  updated_by: string
  updated_at: string
}

export interface SessionPolicyInput {
  idle_timeout: string
  max_lifetime: string
  ip_allowlist: string[]
  require_mfa: boolean
  max_concurrent_sessions: number
  on_limit: SessionOnLimit
}

export interface EffectiveSessionPolicy {
  idle_timeout: string
  max_lifetime: string
  require_mfa: boolean
  max_concurrent_sessions: number
  on_limit: SessionOnLimit
  allowlists: number
  your_address: string
  address_allowed: boolean
}

export const sessionPolicyKeys = {
  all: ['session-policy'] as const,
  list: () => [...sessionPolicyKeys.all, 'list'] as const,
  effective: () => [...sessionPolicyKeys.all, 'effective'] as const,
}

const BASE = '/api/v1/security/session-policies'

export function sessionPoliciesQueryOptions() {
  return queryOptions({
    queryKey: sessionPolicyKeys.list(),
    queryFn: () => ssoRequest<SessionPolicy[]>(BASE),
  })
}

export function effectiveSessionPolicyQueryOptions() {
  return queryOptions({
    queryKey: sessionPolicyKeys.effective(),
    queryFn: () =>
      ssoRequest<EffectiveSessionPolicy>(
        '/api/v1/security/session-policy/effective',
      ),
  })
}

function usePolicyMutation<TVars, TData>(fn: (vars: TVars) => Promise<TData>) {
  const queryClient = useQueryClient()
  return useMutation<TData, ApiError, TVars>({
    mutationFn: fn,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: sessionPolicyKeys.all })
    },
  })
}

export function useSaveSessionPolicy() {
  return usePolicyMutation(
    ({ teamId, input }: { teamId: string; input: SessionPolicyInput }) =>
      ssoRequest<SessionPolicy>(
        `${BASE}/${encodeURIComponent(teamId)}`,
        'PUT',
        input,
      ),
  )
}

export function useClearSessionPolicy() {
  return usePolicyMutation((teamId: string) =>
    ssoRequest<undefined>(`${BASE}/${encodeURIComponent(teamId)}`, 'DELETE'),
  )
}
