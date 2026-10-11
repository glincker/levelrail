import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

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
}

export interface SetDomainAuthGateRequest {
  enabled: boolean
  auth_url: string
  check_path: string
  copy_headers: string[]
  // 0 uses the server default (APP_AUTH_GATE_TIMEOUT).
  timeout_seconds: number
}

export const domainAuthGateKeys = {
  detail: (appName: string, domain: string) =>
    ['apps', appName, 'domains', domain, 'auth-gate'] as const,
}

function authGatePath(appName: string, domain: string): string {
  return `/api/v1/apps/${encodeURIComponent(appName)}/domains/${encodeURIComponent(domain)}/auth-gate`
}

async function readGate(
  res: Response,
  fallback: string,
): Promise<DomainAuthGate> {
  if (!res.ok) {
    throw new ApiError(res.status, await readErrorMessage(res, fallback))
  }
  return (await res.json()) as DomainAuthGate
}

export async function fetchDomainAuthGate(
  appName: string,
  domain: string,
): Promise<DomainAuthGate> {
  const res = await fetch(authGatePath(appName, domain))
  return readGate(res, `fetch domain auth gate failed: ${res.status}`)
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
  return readGate(res, `set domain auth gate failed: ${res.status}`)
}

export async function clearDomainAuthGate(
  appName: string,
  domain: string,
): Promise<DomainAuthGate> {
  const res = await fetch(authGatePath(appName, domain), { method: 'DELETE' })
  return readGate(res, `clear domain auth gate failed: ${res.status}`)
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
