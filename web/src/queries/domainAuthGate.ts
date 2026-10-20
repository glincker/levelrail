import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type AuthGateProvider = 'external' | 'builtin'
export type AuthGateRuleKind = 'user' | 'team' | 'role'

export interface AuthGateRule {
  kind: AuthGateRuleKind
  subject: string
  label: string
}

export interface AuthGateRuleInput {
  kind: AuthGateRuleKind
  subject: string
}

// Mirrors internal/api's domainAuthGateResource.
export interface DomainAuthGate {
  domain: string
  configured: boolean
  enabled: boolean
  auth_url: string
  check_path: string
  copy_headers: string[]
  timeout_seconds: number
  default_timeout_seconds: number
  updated_at?: string
  provider: AuthGateProvider
  allow: AuthGateRule[]
  live_sessions: number
  builtin_available: boolean
}

export interface SetDomainAuthGateRequest {
  enabled: boolean
  auth_url: string
  check_path: string
  copy_headers: string[]
  // 0 uses the server default (APP_AUTH_GATE_TIMEOUT).
  timeout_seconds: number
  provider: AuthGateProvider
  allow: AuthGateRuleInput[]
}

export type AuthGateDecision =
  | 'allowed'
  | 'denied'
  | 'login_redirect'
  | 'token_issued'
  | 'token_exchange'
  | 'session_revoked'

// Mirrors internal/api's authGateEventResource.
export interface AuthGateEvent {
  at: string
  user_id: string
  user_email: string
  decision: AuthGateDecision
  reason: string
  ip: string
  count: number
}

export interface AuthGateEvents {
  domain: string
  since: string
  events: AuthGateEvent[]
}

export interface RevokedAuthGateSessions {
  domain: string
  revoked: number
}

export const domainAuthGateKeys = {
  detail: (appName: string, domain: string) =>
    ['apps', appName, 'domains', domain, 'auth-gate'] as const,
  events: (appName: string, domain: string, since: string) =>
    ['apps', appName, 'domains', domain, 'auth-gate', 'events', since] as const,
}

function authGatePath(appName: string, domain: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/domains/${encodeURIComponent(domain)}/auth-gate`
}

async function readJSON<T>(res: Response, fallback: string): Promise<T> {
  if (!res.ok) {
    throw new ApiError(res.status, await readErrorMessage(res, fallback))
  }
  return (await res.json()) as T
}

export async function fetchDomainAuthGate(
  appName: string,
  domain: string,
): Promise<DomainAuthGate> {
  const res = await fetch(authGatePath(appName, domain))
  return readJSON(res, `fetch domain auth gate failed: ${res.status}`)
}

export function domainAuthGateQueryOptions(appName: string, domain: string) {
  return queryOptions({
    queryKey: domainAuthGateKeys.detail(appName, domain),
    queryFn: () => fetchDomainAuthGate(appName, domain),
    enabled: domain.length > 0,
    staleTime: 30_000,
  })
}

export function useDomainAuthGate(appName: string, domain: string) {
  return useQuery(domainAuthGateQueryOptions(appName, domain))
}

export async function setDomainAuthGate(
  appName: string,
  domain: string,
  req: SetDomainAuthGateRequest,
): Promise<DomainAuthGate> {
  const res = await fetch(authGatePath(appName, domain), {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  return readJSON(res, `set domain auth gate failed: ${res.status}`)
}

export async function clearDomainAuthGate(
  appName: string,
  domain: string,
): Promise<DomainAuthGate> {
  const res = await fetch(authGatePath(appName, domain), { method: 'DELETE' })
  return readJSON(res, `clear domain auth gate failed: ${res.status}`)
}

export function useSetDomainAuthGate(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainAuthGate, ApiError, SetDomainAuthGateRequest>({
    mutationFn: (req) => setDomainAuthGate(appName, domain, req),
    onSuccess: (updated) => {
      queryClient.setQueryData(
        domainAuthGateKeys.detail(appName, domain),
        updated,
      )
    },
  })
}

export function useClearDomainAuthGate(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<DomainAuthGate, ApiError, void>({
    mutationFn: () => clearDomainAuthGate(appName, domain),
    onSuccess: (updated) => {
      queryClient.setQueryData(
        domainAuthGateKeys.detail(appName, domain),
        updated,
      )
    },
  })
}

export async function fetchAuthGateEvents(
  appName: string,
  domain: string,
  since: string,
): Promise<AuthGateEvents> {
  const res = await fetch(
    `${authGatePath(appName, domain)}/events?since=${encodeURIComponent(since)}`,
  )
  return readJSON(res, `fetch access log failed: ${res.status}`)
}

export function useAuthGateEvents(
  appName: string,
  domain: string,
  since: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: domainAuthGateKeys.events(appName, domain, since),
    queryFn: () => fetchAuthGateEvents(appName, domain, since),
    enabled: enabled && domain.length > 0,
    staleTime: 15_000,
  })
}

export async function revokeAuthGateSessions(
  appName: string,
  domain: string,
): Promise<RevokedAuthGateSessions> {
  const res = await fetch(`${authGatePath(appName, domain)}/sessions`, {
    method: 'DELETE',
  })
  return readJSON(res, `sign out visitors failed: ${res.status}`)
}

export function useRevokeAuthGateSessions(appName: string, domain: string) {
  const queryClient = useQueryClient()
  return useMutation<RevokedAuthGateSessions, ApiError, void>({
    mutationFn: () => revokeAuthGateSessions(appName, domain),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: domainAuthGateKeys.detail(appName, domain),
      })
    },
  })
}
