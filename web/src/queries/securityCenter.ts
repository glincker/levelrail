import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { TrustedDevice } from './signIn'

// Security center (internal/api/security_*.go).

export type PostureSeverity = 'critical' | 'high' | 'medium' | 'low'
export type PostureStatus = 'pass' | 'fail' | 'unknown'

export interface PostureFix {
  kind: 'link' | 'action'
  link?: string
  action?: string
  params?: Record<string, string>
  cli?: string
}

export interface PostureItem {
  id: string
  severity: PostureSeverity
  status: PostureStatus
  count: number
  subjects?: string[]
  detail?: string
  fix?: PostureFix
}

export interface PostureCounts {
  critical: number
  high: number
  medium: number
  low: number
  unknown: number
  passing: number
}

export interface SecurityPosture {
  score: number
  grade: string
  generated_at: string
  counts: PostureCounts
  full: boolean
  items: PostureItem[]
  account: PostureItem[]
}

export type ApprovalScope = 'password_only' | 'all_methods'

export interface SecurityPolicy {
  approval_scope: ApprovalScope
  max_token_lifetime_days: number
  warn_unused_days: number
  disable_unused_days: number
  sources: Record<string, string>
  new_device_approval: boolean
  notice_grace_days: number
}

export type SecurityPolicyUpdate = Partial<
  Pick<
    SecurityPolicy,
    | 'approval_scope'
    | 'max_token_lifetime_days'
    | 'warn_unused_days'
    | 'disable_unused_days'
  >
>

export interface SessionEntry {
  id: string
  browser: string
  ip: string
  network: string
  created_at: string
  last_seen_at: string
  expires_at: string
  current: boolean
}

export interface SessionToken {
  id: string
  name: string
  abilities: string[]
  created_at: string
  last_used_at?: string
  expires_at?: string
}

export interface SecuritySessions {
  user_id: string
  self: boolean
  sessions: SessionEntry[]
  trusted_devices: TrustedDevice[]
  tokens: SessionToken[]
}

export interface AccountSecurity {
  require_new_device_approval: boolean
  reset_flagged: boolean
  reset_flagged_at?: string
  reset_flag_reason?: string
}

export const securityKeys = {
  posture: ['security', 'posture'] as const,
  policy: ['security', 'policy'] as const,
  account: ['security', 'account'] as const,
  sessions: (userId: string) => ['security', 'sessions', userId] as const,
}

async function send<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const payload = body === undefined && method === 'POST' ? {} : body
  const res = await fetch(path, {
    method,
    headers:
      payload === undefined
        ? undefined
        : { 'Content-Type': 'application/json' },
    body: payload === undefined ? undefined : JSON.stringify(payload),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${path} failed: ${res.status}`),
    )
  }
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

function withUser(path: string, userId: string): string {
  return userId === '' ? path : `${path}?user_id=${encodeURIComponent(userId)}`
}

export function postureQueryOptions() {
  return queryOptions({
    queryKey: securityKeys.posture,
    queryFn: () => send<SecurityPosture>('GET', '/api/v1/security/posture'),
  })
}

export function securityPolicyQueryOptions() {
  return queryOptions({
    queryKey: securityKeys.policy,
    queryFn: () => send<SecurityPolicy>('GET', '/api/v1/security/policy'),
  })
}

export function accountSecurityQueryOptions() {
  return queryOptions({
    queryKey: securityKeys.account,
    queryFn: () => send<AccountSecurity>('GET', '/api/v1/security/account'),
  })
}

export function securitySessionsQueryOptions(userId: string) {
  return queryOptions({
    queryKey: securityKeys.sessions(userId),
    queryFn: () =>
      send<SecuritySessions>(
        'GET',
        withUser('/api/v1/security/sessions', userId),
      ),
  })
}

function useInvalidateSecurity() {
  const qc = useQueryClient()
  return () => qc.invalidateQueries({ queryKey: ['security'] })
}

export function useUpdateSecurityPolicy() {
  const invalidate = useInvalidateSecurity()
  return useMutation<SecurityPolicy, ApiError, SecurityPolicyUpdate>({
    mutationFn: (body) =>
      send<SecurityPolicy>('PUT', '/api/v1/security/policy', body),
    onSettled: invalidate,
  })
}

export function useUpdateAccountSecurity() {
  const invalidate = useInvalidateSecurity()
  return useMutation<AccountSecurity, ApiError, boolean>({
    mutationFn: (on) =>
      send<AccountSecurity>('PUT', '/api/v1/security/account', {
        require_new_device_approval: on,
      }),
    onSettled: invalidate,
  })
}

export function useRevokeSession() {
  const invalidate = useInvalidateSecurity()
  return useMutation<undefined, ApiError, { id: string; userId: string }>({
    mutationFn: ({ id, userId }) =>
      send(
        'DELETE',
        withUser(`/api/v1/security/sessions/${encodeURIComponent(id)}`, userId),
      ),
    onSettled: invalidate,
  })
}

export function useRevokeOtherSessions() {
  const invalidate = useInvalidateSecurity()
  return useMutation<{ revoked: number }, ApiError, string>({
    mutationFn: (userId) =>
      send('POST', withUser('/api/v1/security/sessions/revoke-others', userId)),
    onSettled: invalidate,
  })
}

export function disownSignIn(token: string) {
  return send<{ session_revoked: boolean }>(
    'POST',
    '/api/v1/auth/sign-in-alert/disown',
    { token },
  )
}

export function useDisownSignIn() {
  return useMutation<{ session_revoked: boolean }, ApiError, string>({
    mutationFn: disownSignIn,
  })
}
