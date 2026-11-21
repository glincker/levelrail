import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { apiRequest } from '../lib/apiRequest'
import { parseRetryAfter } from '../lib/cloudSpend'
import type {
  AwsControlIdentity,
  AwsLinkKit,
  AwsLinkKitInput,
  AwsSsoPoll,
  AwsSsoSelectInput,
  AwsSsoStart,
  AwsVaultKit,
  AwsVaultKitInput,
  CreateAwsVaultAccountInput,
  CloudAccessData,
  CloudAccount,
  CloudCapabilityResult,
  CloudGrant,
  CloudGrantInput,
  CloudInstance,
  CloudInstanceAction,
  CloudInstanceDetail,
  CloudRegion,
  CloudResizeInput,
  CloudSpendReport,
  CloudVerifyResult,
  CreateAwsRoleAccountInput,
  OracleSessionCheck,
  OracleSessionInput,
} from '../types/cloudAccounts'

const BASE = '/api/v1/cloud-accounts'

export const cloudAccountKeys = {
  all: ['cloud-accounts'] as const,
  list: () => [...cloudAccountKeys.all, 'list'] as const,
  detail: (id: string) => [...cloudAccountKeys.all, 'detail', id] as const,
  regions: (id: string) => [...cloudAccountKeys.all, 'regions', id] as const,
  grants: (id: string) => [...cloudAccountKeys.all, 'grants', id] as const,
  instances: (id: string) =>
    [...cloudAccountKeys.all, 'instances', id] as const,
  instance: (id: string, iid: string) =>
    [...cloudAccountKeys.instances(id), iid] as const,
  spend: (id: string) => [...cloudAccountKeys.all, 'spend', id] as const,
  access: () => [...cloudAccountKeys.all, 'me-access'] as const,
  controlIdentity: () =>
    [...cloudAccountKeys.all, 'aws-control-identity'] as const,
}

const enc = encodeURIComponent

export function cloudAccountsQueryOptions() {
  return queryOptions({
    queryKey: cloudAccountKeys.list(),
    queryFn: async () =>
      (await apiRequest<CloudAccount[] | null>(BASE, {
        fallbackMessage: 'load cloud accounts failed',
      })) ?? [],
  })
}

export function useCloudAccounts() {
  return useQuery(cloudAccountsQueryOptions())
}

export function useCloudAccount(id: string) {
  return useQuery({
    queryKey: cloudAccountKeys.detail(id),
    queryFn: () =>
      apiRequest<CloudAccount>(`${BASE}/${enc(id)}`, {
        fallbackMessage: 'load cloud account failed',
      }),
  })
}

export function useCloudAccountRegions(id: string, enabled: boolean) {
  return useQuery({
    queryKey: cloudAccountKeys.regions(id),
    enabled,
    retry: false,
    queryFn: async () =>
      (await apiRequest<CloudRegion[] | null>(`${BASE}/${enc(id)}/regions`, {
        fallbackMessage: 'load regions failed',
      })) ?? [],
  })
}

export function useCloudGrants(id: string, enabled: boolean) {
  return useQuery({
    queryKey: cloudAccountKeys.grants(id),
    enabled,
    retry: false,
    queryFn: async () =>
      (await apiRequest<CloudGrant[] | null>(`${BASE}/${enc(id)}/grants`, {
        fallbackMessage: 'load grants failed',
      })) ?? [],
  })
}

export function useAwsLinkKit() {
  return useMutation({
    mutationFn: (input: AwsLinkKitInput) =>
      apiRequest<AwsLinkKit>(`${BASE}/aws/link-kit`, {
        method: 'POST',
        body: input,
        fallbackMessage: 'generate link kit failed',
      }),
  })
}

export function useAwsVaultKit() {
  return useMutation({
    mutationFn: (input: AwsVaultKitInput) =>
      apiRequest<AwsVaultKit>(`${BASE}/aws/vault-kit`, {
        method: 'POST',
        body: input,
        fallbackMessage: 'generate vault kit failed',
      }),
  })
}

export function useCreateAwsVaultAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateAwsVaultAccountInput) =>
      apiRequest<CloudAccount>(BASE, {
        method: 'POST',
        body: { provider: 'aws', method: 'vault_aws', ...input },
        fallbackMessage: 'create cloud account failed',
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: cloudAccountKeys.all }),
  })
}

export function useCreateAwsRoleAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateAwsRoleAccountInput) =>
      apiRequest<CloudAccount>(BASE, {
        method: 'POST',
        body: { provider: 'aws', method: 'role_assumption', ...input },
        fallbackMessage: 'create cloud account failed',
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: cloudAccountKeys.all }),
  })
}

// Offline check of pasted session files; nothing is stored.
export function useImportOracleSession() {
  return useMutation({
    mutationFn: (input: OracleSessionInput) =>
      apiRequest<OracleSessionCheck>(`${BASE}/oracle/session-import`, {
        method: 'POST',
        body: input,
        fallbackMessage: 'check oracle session failed',
      }),
  })
}

export function useCreateOracleSessionAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: OracleSessionInput & { label?: string }) =>
      apiRequest<CloudAccount>(BASE, {
        method: 'POST',
        body: {
          provider: 'oracle',
          method: 'oci_session',
          verify: true,
          ...input,
        },
        fallbackMessage: 'create cloud account failed',
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: cloudAccountKeys.all }),
  })
}

export function useReloginOracleSession(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { session_token: string; private_key_pem: string }) =>
      apiRequest<CloudAccount>(`${BASE}/${enc(id)}`, {
        method: 'PATCH',
        body: input,
        fallbackMessage: 'relogin cloud account failed',
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: cloudAccountKeys.all }),
  })
}

export function useVerifyCloudAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      apiRequest<CloudVerifyResult>(`${BASE}/${enc(id)}/verify`, {
        method: 'POST',
        fallbackMessage: 'verify cloud account failed',
      }),
    onSettled: () => qc.invalidateQueries({ queryKey: cloudAccountKeys.all }),
  })
}

export function useProbeCloudAccount() {
  return useMutation({
    mutationFn: ({
      id,
      capabilities,
    }: {
      id: string
      capabilities: string[]
    }) =>
      apiRequest<{ capabilities: CloudCapabilityResult[] }>(
        `${BASE}/${enc(id)}/probe`,
        {
          method: 'POST',
          body: { capabilities },
          fallbackMessage: 'probe cloud account failed',
        },
      ),
  })
}

export function useDeleteCloudAccount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      apiRequest<null>(`${BASE}/${enc(id)}`, {
        method: 'DELETE',
        fallbackMessage: 'delete cloud account failed',
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: cloudAccountKeys.all }),
  })
}

export function useCreateCloudGrant(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CloudGrantInput) =>
      apiRequest<CloudGrant>(`${BASE}/${enc(id)}/grants`, {
        method: 'POST',
        body: input,
        fallbackMessage: 'add grant failed',
      }),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: cloudAccountKeys.grants(id) }),
  })
}

export function useRevokeCloudGrant(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (gid: string) =>
      apiRequest<null>(`${BASE}/${enc(id)}/grants/${enc(gid)}`, {
        method: 'DELETE',
        fallbackMessage: 'revoke grant failed',
      }),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: cloudAccountKeys.grants(id) }),
  })
}

export function useCloudAccess() {
  return useQuery({
    queryKey: cloudAccountKeys.access(),
    retry: false,
    staleTime: 30_000,
    queryFn: () =>
      apiRequest<CloudAccessData>('/api/v1/me/cloud-access', {
        fallbackMessage: 'load cloud access failed',
      }),
  })
}

export function useAwsControlIdentity(enabled: boolean) {
  return useQuery({
    queryKey: cloudAccountKeys.controlIdentity(),
    enabled,
    retry: false,
    staleTime: 60_000,
    queryFn: () =>
      apiRequest<AwsControlIdentity>(`${BASE}/aws/control-identity`, {
        fallbackMessage: 'load control identity failed',
      }),
  })
}

export function useCloudInstances(id: string, enabled: boolean) {
  return useQuery({
    queryKey: cloudAccountKeys.instances(id),
    enabled,
    retry: false,
    queryFn: async () =>
      (await apiRequest<CloudInstance[] | null>(
        `${BASE}/${enc(id)}/instances`,
        {
          fallbackMessage: 'load instances failed',
        },
      )) ?? [],
  })
}

export function useCloudInstance(id: string, iid: string | null) {
  return useQuery({
    queryKey: cloudAccountKeys.instance(id, iid ?? ''),
    enabled: iid !== null,
    retry: false,
    queryFn: () =>
      apiRequest<CloudInstanceDetail>(
        `${BASE}/${enc(id)}/instances/${enc(iid ?? '')}`,
        { fallbackMessage: 'load instance failed' },
      ),
  })
}

export function useCloudInstanceAction(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      iid,
      action,
    }: {
      iid: string
      action: CloudInstanceAction
    }) =>
      apiRequest<{ status: string }>(
        `${BASE}/${enc(id)}/instances/${enc(iid)}/actions`,
        {
          method: 'POST',
          body: { action },
          fallbackMessage: 'instance action failed',
        },
      ),
    onSettled: () =>
      qc.invalidateQueries({ queryKey: cloudAccountKeys.instances(id) }),
  })
}

export function useSetInstanceTags(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      iid,
      tags,
    }: {
      iid: string
      tags: Record<string, string>
    }) =>
      apiRequest<{ status: string }>(
        `${BASE}/${enc(id)}/instances/${enc(iid)}/tags`,
        { method: 'PUT', body: { tags }, fallbackMessage: 'save tags failed' },
      ),
    onSettled: () =>
      qc.invalidateQueries({ queryKey: cloudAccountKeys.instances(id) }),
  })
}

export function useResizeInstance(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ iid, spec }: { iid: string; spec: CloudResizeInput }) =>
      apiRequest<{ status: string }>(
        `${BASE}/${enc(id)}/instances/${enc(iid)}/resize`,
        { method: 'POST', body: spec, fallbackMessage: 'resize failed' },
      ),
    onSettled: () =>
      qc.invalidateQueries({ queryKey: cloudAccountKeys.instances(id) }),
  })
}

export function useTerminateInstance(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({
      iid,
      keepBootVolume,
    }: {
      iid: string
      keepBootVolume: boolean
    }) =>
      apiRequest<null>(
        `${BASE}/${enc(id)}/instances/${enc(iid)}?keep_boot_volume=${String(keepBootVolume)}`,
        { method: 'DELETE', fallbackMessage: 'terminate failed' },
      ),
    onSettled: () =>
      qc.invalidateQueries({ queryKey: cloudAccountKeys.instances(id) }),
  })
}

export function useCloudSpend(id: string, enabled: boolean) {
  return useQuery({
    queryKey: cloudAccountKeys.spend(id),
    enabled,
    retry: false,
    queryFn: () =>
      apiRequest<CloudSpendReport>(`${BASE}/${enc(id)}/spend`, {
        fallbackMessage: 'load spend failed',
      }),
  })
}

/** Carries the Retry-After seconds a 429 refresh answered with. */
export class SpendRefreshLimited extends Error {
  readonly retryAfterSeconds: number | null
  constructor(retryAfterSeconds: number | null) {
    super('spend refresh limited')
    this.name = 'SpendRefreshLimited'
    this.retryAfterSeconds = retryAfterSeconds
  }
}

export function useRefreshCloudSpend(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (): Promise<CloudSpendReport> => {
      const res = await fetch(`${BASE}/${enc(id)}/spend/refresh`, {
        method: 'POST',
      })
      if (res.status === 429) {
        throw new SpendRefreshLimited(
          parseRetryAfter(res.headers.get('Retry-After')),
        )
      }
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `refresh spend failed: ${res.status}`),
        )
      }
      return (await res.json()) as CloudSpendReport
    },
    onSuccess: (rep) => qc.setQueryData(cloudAccountKeys.spend(id), rep),
  })
}

export interface DigitalOceanOAuthStatus {
  enabled: boolean
  redirect_uri?: string
  hint?: string
}

export function useDigitalOceanOAuthStatus(enabled: boolean) {
  return useQuery({
    queryKey: [...cloudAccountKeys.all, 'do-oauth-status'] as const,
    enabled,
    retry: false,
    queryFn: () =>
      apiRequest<DigitalOceanOAuthStatus>(`${BASE}/oauth/digitalocean/status`, {
        fallbackMessage: 'load digitalocean oauth status failed',
      }),
  })
}

export function useStartDigitalOceanOAuth() {
  return useMutation({
    mutationFn: () =>
      apiRequest<{ url: string }>(`${BASE}/oauth/digitalocean/start`, {
        fallbackMessage: 'start digitalocean oauth failed',
      }),
  })
}

const SSO = `${BASE}/aws/sso`

export function useAwsSsoStart() {
  return useMutation({
    mutationFn: (input: { start_url: string; region: string }) =>
      apiRequest<AwsSsoStart>(`${SSO}/start`, {
        method: 'POST',
        body: input,
        fallbackMessage: 'start aws sso login failed',
      }),
  })
}

export function useAwsSsoRelogin() {
  return useMutation({
    mutationFn: (accountId: string) =>
      apiRequest<AwsSsoStart>(`${SSO}/relogin`, {
        method: 'POST',
        body: { account_id: accountId },
        fallbackMessage: 'start aws sso login failed',
      }),
  })
}

export function useAwsSsoPoll() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (sessionId: string) =>
      apiRequest<AwsSsoPoll>(`${SSO}/poll`, {
        method: 'POST',
        body: { session_id: sessionId },
        fallbackMessage: 'check aws sso login failed',
      }),
    onSuccess: (r) => {
      if (r.account_id)
        void qc.invalidateQueries({ queryKey: cloudAccountKeys.all })
    },
  })
}

export function useAwsSsoSelect() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: AwsSsoSelectInput) =>
      apiRequest<CloudAccount>(`${SSO}/select`, {
        method: 'POST',
        body: input,
        fallbackMessage: 'link aws sso account failed',
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: cloudAccountKeys.all }),
  })
}
