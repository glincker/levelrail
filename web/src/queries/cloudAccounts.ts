import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { apiRequest } from '../lib/apiRequest'
import type {
  AwsLinkKit,
  AwsLinkKitInput,
  CloudAccount,
  CloudCapabilityResult,
  CloudGrant,
  CloudGrantInput,
  CloudRegion,
  CloudVerifyResult,
  CreateAwsRoleAccountInput,
} from '../types/cloudAccounts'

const BASE = '/api/v1/cloud-accounts'

export const cloudAccountKeys = {
  all: ['cloud-accounts'] as const,
  list: () => [...cloudAccountKeys.all, 'list'] as const,
  detail: (id: string) => [...cloudAccountKeys.all, 'detail', id] as const,
  regions: (id: string) => [...cloudAccountKeys.all, 'regions', id] as const,
  grants: (id: string) => [...cloudAccountKeys.all, 'grants', id] as const,
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
