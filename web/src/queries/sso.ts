// Fetchers and hooks for internal/api/sso_*.go: SAML connections, group to
// role mappings and SSO enforcement (root only), plus the public provider list.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { SSOTestResult } from './ssoOidc'

export interface SAMLCertificateInfo {
  subject: string
  not_after: string
}

export interface SAMLConnection {
  id: string
  name: string
  enabled: boolean
  configured: boolean
  idp_entity_id: string
  idp_sso_url: string
  idp_certificates: string
  idp_metadata_url: string
  certificates: SAMLCertificateInfo[]
  email_attribute: string
  name_attribute: string
  groups_attribute: string
  allowed_domains: string[]
  jit_provisioning: boolean
  default_role_id: string
  sync_roles: boolean
  sp_entity_id: string
  sp_acs_url: string
  sp_metadata_url: string
  created_at: string
  updated_at: string
}

export type SAMLConnectionInput = Omit<
  SAMLConnection,
  | 'id'
  | 'configured'
  | 'certificates'
  | 'sp_entity_id'
  | 'sp_acs_url'
  | 'sp_metadata_url'
  | 'created_at'
  | 'updated_at'
>

export interface SSOGroupMapping {
  group: string
  role_id: string
  priority: number
}

export interface SSOSettings {
  enforce_sso: boolean
  sp_certificate: string
  base_url: string
  base_url_error?: string
  master_key_missing: boolean
  scim_base_url: string
  scim_default_role_id: string
}

export interface SCIMToken {
  id: string
  name: string
  prefix: string
  created_by: string
  created_at: string
  last_used_at: string | null
  revoked_at: string | null
}

export interface SCIMTokenCreated extends SCIMToken {
  token: string
}

export interface PublicSAMLProvider {
  id: string
  name: string
}

export const ssoKeys = {
  all: ['sso'] as const,
  connections: () => [...ssoKeys.all, 'connections'] as const,
  mappings: () => [...ssoKeys.all, 'mappings'] as const,
  settings: () => [...ssoKeys.all, 'settings'] as const,
  publicProviders: () => [...ssoKeys.all, 'public'] as const,
  scimTokens: () => [...ssoKeys.all, 'scim-tokens'] as const,
}

export async function ssoRequest<T>(
  url: string,
  method: 'GET' | 'POST' | 'PUT' | 'DELETE' = 'GET',
  body?: object,
): Promise<T> {
  const res = await fetch(url, {
    method,
    headers: body ? { 'Content-Type': 'application/json' } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${method} ${url} failed: ${res.status}`),
    )
  }
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

const CONNECTIONS = '/api/v1/sso/saml/connections'

export function samlConnectionsQueryOptions() {
  return queryOptions({
    queryKey: ssoKeys.connections(),
    queryFn: () => ssoRequest<SAMLConnection[]>(CONNECTIONS),
  })
}

export function ssoMappingsQueryOptions() {
  return queryOptions({
    queryKey: ssoKeys.mappings(),
    queryFn: () =>
      ssoRequest<{ mappings: SSOGroupMapping[] }>('/api/v1/sso/group-mappings'),
  })
}

export function ssoSettingsQueryOptions() {
  return queryOptions({
    queryKey: ssoKeys.settings(),
    queryFn: () => ssoRequest<SSOSettings>('/api/v1/sso/settings'),
  })
}

// Public and unauthenticated: the login screen shows one button per entry.
export function usePublicSAMLProviders(enabled = true) {
  return useQuery({
    enabled,
    queryKey: ssoKeys.publicProviders(),
    queryFn: async () => {
      const body = await ssoRequest<unknown>('/api/v1/auth/saml/providers')
      return Array.isArray(body) ? (body as PublicSAMLProvider[]) : []
    },
    staleTime: 60_000,
  })
}

function useSSOMutation<TVars, TData>(fn: (vars: TVars) => Promise<TData>) {
  const queryClient = useQueryClient()
  return useMutation<TData, ApiError, TVars>({
    mutationFn: fn,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ssoKeys.all })
    },
  })
}

export function useSaveSAMLConnection() {
  return useSSOMutation(
    ({ id, input }: { id?: string; input: SAMLConnectionInput }) =>
      id
        ? ssoRequest<SAMLConnection>(
            `${CONNECTIONS}/${encodeURIComponent(id)}`,
            'PUT',
            input,
          )
        : ssoRequest<SAMLConnection>(CONNECTIONS, 'POST', input),
  )
}

export function samlTestStartURL(id: string): string {
  return `${CONNECTIONS}/${encodeURIComponent(id)}/test/start`
}

// One-shot: the server forgets a result once it has been read.
export function fetchSAMLTestResult(id: string): Promise<SSOTestResult> {
  return ssoRequest<SSOTestResult>(
    `${CONNECTIONS}/${encodeURIComponent(id)}/test/result`,
  )
}

export function useDeleteSAMLConnection() {
  return useSSOMutation((id: string) =>
    ssoRequest<undefined>(`${CONNECTIONS}/${encodeURIComponent(id)}`, 'DELETE'),
  )
}

export function useImportSAMLMetadata() {
  return useSSOMutation(
    ({ id, url, xml }: { id: string; url?: string; xml?: string }) =>
      ssoRequest<SAMLConnection>(
        `${CONNECTIONS}/${encodeURIComponent(id)}/metadata`,
        'POST',
        url ? { url } : { xml },
      ),
  )
}

export function useSaveSSOMappings() {
  return useSSOMutation((mappings: SSOGroupMapping[]) =>
    ssoRequest<{ mappings: SSOGroupMapping[] }>(
      '/api/v1/sso/group-mappings',
      'PUT',
      { mappings },
    ),
  )
}

export function useSetSSOEnforce() {
  return useSSOMutation((enforce: boolean) =>
    ssoRequest<SSOSettings>('/api/v1/sso/settings', 'PUT', {
      enforce_sso: enforce,
    }),
  )
}

export function connectionInput(c: SAMLConnection): SAMLConnectionInput {
  return {
    name: c.name,
    enabled: c.enabled,
    idp_entity_id: c.idp_entity_id,
    idp_sso_url: c.idp_sso_url,
    idp_certificates: c.idp_certificates,
    idp_metadata_url: c.idp_metadata_url,
    email_attribute: c.email_attribute,
    name_attribute: c.name_attribute,
    groups_attribute: c.groups_attribute,
    allowed_domains: c.allowed_domains,
    jit_provisioning: c.jit_provisioning,
    default_role_id: c.default_role_id,
    sync_roles: c.sync_roles,
  }
}

const SCIM_TOKENS = '/api/v1/sso/scim/tokens'

export function scimTokensQueryOptions() {
  return queryOptions({
    queryKey: ssoKeys.scimTokens(),
    queryFn: () => ssoRequest<SCIMToken[]>(SCIM_TOKENS),
  })
}

export function useCreateSCIMToken() {
  return useSSOMutation((name: string) =>
    ssoRequest<SCIMTokenCreated>(SCIM_TOKENS, 'POST', { name }),
  )
}

export function useRotateSCIMToken() {
  return useSSOMutation((id: string) =>
    ssoRequest<SCIMTokenCreated>(
      `${SCIM_TOKENS}/${encodeURIComponent(id)}/rotate`,
      'POST',
    ),
  )
}

export function useRevokeSCIMToken() {
  return useSSOMutation((id: string) =>
    ssoRequest<undefined>(`${SCIM_TOKENS}/${encodeURIComponent(id)}`, 'DELETE'),
  )
}

export function useSetSCIMDefaultRole() {
  return useSSOMutation((roleID: string) =>
    ssoRequest<SSOSettings>('/api/v1/sso/settings', 'PUT', {
      scim_default_role_id: roleID,
    }),
  )
}
