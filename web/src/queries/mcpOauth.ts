// Consent and pre-registered client calls for remote MCP OAuth
// (internal/api/mcp_oauth_authorize.go and mcp_oauth_clients.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface McpConsent {
  id: string
  client_id: string
  client_name: string
  redirect_host: string
  requested_scopes: string[]
  grantable_scopes: string[]
  resource: string
  access_ttl_seconds: number
  refresh_ttl_seconds: number
  expires_at: string
}

export interface McpClient {
  id: string
  name: string
  redirect_uris: string[]
  scopes: string[]
  created_at: string
}

export interface McpClientInput {
  name: string
  redirect_uris: string[]
}

export const mcpOauthKeys = {
  all: ['mcp-oauth'] as const,
  consent: (id: string) => [...mcpOauthKeys.all, 'consent', id] as const,
  clients: () => [...mcpOauthKeys.all, 'clients'] as const,
}

async function request<T>(
  path: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

function jsonInit(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  }
}

const consentBase = '/api/v1/mcp/oauth/consent'
const clientsBase = '/api/v1/mcp/oauth/clients'

export function useMcpConsent(id: string) {
  return useQuery(
    queryOptions({
      queryKey: mcpOauthKeys.consent(id),
      queryFn: () =>
        request<McpConsent>(
          `${consentBase}/${encodeURIComponent(id)}`,
          undefined,
          'load consent request',
        ),
      retry: false,
      staleTime: Infinity,
    }),
  )
}

export function useDecideMcpConsent(id: string) {
  return useMutation({
    mutationFn: ({ approve, scopes }: { approve: boolean; scopes: string[] }) =>
      request<{ redirect_to: string }>(
        `${consentBase}/${encodeURIComponent(id)}/${approve ? 'approve' : 'deny'}`,
        jsonInit('POST', approve ? { scopes } : {}),
        approve ? 'approve access' : 'deny access',
      ),
  })
}

export function useMcpClients(enabled: boolean) {
  return useQuery(
    queryOptions({
      queryKey: mcpOauthKeys.clients(),
      queryFn: () =>
        request<McpClient[]>(clientsBase, undefined, 'list AI clients'),
      enabled,
      retry: false,
    }),
  )
}

export function useCreateMcpClient() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: McpClientInput) =>
      request<McpClient>(clientsBase, jsonInit('POST', input), 'add AI client'),
    onSuccess: () => qc.invalidateQueries({ queryKey: mcpOauthKeys.clients() }),
  })
}

export function useDeleteMcpClient() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      request<void>(
        `${clientsBase}/${encodeURIComponent(id)}`,
        { method: 'DELETE' },
        'remove AI client',
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: mcpOauthKeys.clients() }),
  })
}
