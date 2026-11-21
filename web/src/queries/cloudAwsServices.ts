import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '../lib/apiRequest'
import { cloudAccountKeys } from './cloudAccounts'
import type {
  AwsAcmCert,
  AwsAuditEvent,
  AwsCleanupPlan,
  AwsEcrLoginToken,
  AwsEcrRepo,
  AwsGuardStatus,
  AwsSecretsList,
} from '../types/cloudAccounts'

const enc = encodeURIComponent
const base = (id: string) => `/api/v1/cloud-accounts/${enc(id)}/aws`

const awsKey = (id: string) => [...cloudAccountKeys.all, 'aws', id] as const

export const cloudAwsKeys = {
  all: awsKey,
  guard: (id: string) => [...awsKey(id), 'guard'] as const,
  ecr: (id: string) => [...awsKey(id), 'ecr'] as const,
  acm: (id: string) => [...awsKey(id), 'acm'] as const,
  secrets: (id: string) => [...awsKey(id), 'secrets'] as const,
  audit: (id: string, resource: string) =>
    [...awsKey(id), 'audit', resource] as const,
  cleanup: (id: string) => [...awsKey(id), 'cleanup'] as const,
}

export function useAwsGuard(id: string) {
  return useQuery({
    queryKey: cloudAwsKeys.guard(id),
    retry: false,
    queryFn: () =>
      apiRequest<AwsGuardStatus>(`${base(id)}/guard`, {
        fallbackMessage: 'load guard failed',
      }),
  })
}

export function useAwsEcr(id: string, enabled: boolean) {
  return useQuery({
    queryKey: cloudAwsKeys.ecr(id),
    enabled,
    retry: false,
    queryFn: async () =>
      (
        await apiRequest<{ repositories: AwsEcrRepo[] | null }>(
          `${base(id)}/ecr/repositories`,
          { fallbackMessage: 'load repositories failed' },
        )
      ).repositories ?? [],
  })
}

export function useCreateEcrRepo(id: string) {
  const qc = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (name: string) =>
      apiRequest<unknown>(`${base(id)}/ecr/repositories`, {
        method: 'POST',
        body: { name },
        fallbackMessage: 'create repository failed',
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: cloudAwsKeys.ecr(id) }),
  })
}

export function useEcrLoginToken(id: string) {
  return useMutation({
    meta: { errorToast: false },
    mutationFn: () =>
      apiRequest<AwsEcrLoginToken>(`${base(id)}/ecr/login-token`, {
        method: 'POST',
        fallbackMessage: 'registry login failed',
      }),
  })
}

export function useAwsAcm(id: string, enabled: boolean) {
  return useQuery({
    queryKey: cloudAwsKeys.acm(id),
    enabled,
    retry: false,
    queryFn: async () =>
      (
        await apiRequest<{ certificates: AwsAcmCert[] | null }>(
          `${base(id)}/acm/certificates`,
          { fallbackMessage: 'load certificates failed' },
        )
      ).certificates ?? [],
  })
}

export function useRequestAcmCert(id: string) {
  const qc = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (domain: string) =>
      apiRequest<unknown>(`${base(id)}/acm/certificates`, {
        method: 'POST',
        body: { domain },
        fallbackMessage: 'request certificate failed',
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: cloudAwsKeys.acm(id) }),
  })
}

export function useAwsSecrets(id: string, enabled: boolean) {
  return useQuery({
    queryKey: cloudAwsKeys.secrets(id),
    enabled,
    retry: false,
    queryFn: () =>
      apiRequest<AwsSecretsList>(`${base(id)}/secrets`, {
        fallbackMessage: 'load parameters failed',
      }),
  })
}

export function usePutAwsSecret(id: string) {
  const qc = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (v: { name: string; value: string; overwrite: boolean }) =>
      apiRequest<unknown>(`${base(id)}/secrets`, {
        method: 'POST',
        body: v,
        fallbackMessage: 'save parameter failed',
      }),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: cloudAwsKeys.secrets(id) }),
  })
}

export function useAwsAudit(id: string, resource: string, enabled: boolean) {
  return useQuery({
    queryKey: cloudAwsKeys.audit(id, resource),
    enabled: enabled && resource !== '',
    retry: false,
    queryFn: async () =>
      (
        await apiRequest<{ events: AwsAuditEvent[] | null }>(
          `${base(id)}/audit?resource=${enc(resource)}`,
          { fallbackMessage: 'load audit events failed' },
        )
      ).events ?? [],
  })
}

export function useAwsCleanupPlan(id: string, enabled: boolean) {
  return useQuery({
    queryKey: cloudAwsKeys.cleanup(id),
    enabled,
    retry: false,
    queryFn: () =>
      apiRequest<AwsCleanupPlan>(`${base(id)}/cleanup/plan`, {
        fallbackMessage: 'load cleanup plan failed',
      }),
  })
}

export function useSetAwsKillSwitch(id: string) {
  const qc = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (enabled: boolean) =>
      apiRequest<{ kill_switch: boolean }>(`${base(id)}/guard/kill-switch`, {
        method: 'POST',
        body: { enabled },
        fallbackMessage: 'save kill switch failed',
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: cloudAwsKeys.guard(id) }),
  })
}
