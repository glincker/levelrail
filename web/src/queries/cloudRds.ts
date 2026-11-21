import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '../lib/apiRequest'
import { cloudAccountKeys } from './cloudAccounts'

export interface AwsRdsInstance {
  id: string
  engine: string
  class: string
  status: string
  host: string
  port: number
  allocated_gb: number
  multi_az: boolean
  managed: boolean
  est_monthly_usd: number | null
}

export interface AwsRdsEstimate {
  est_monthly_usd: number | null
  class_allowed: boolean
  needs_confirm: boolean
  per_create_max_usd: number
  default_expires_hours: number
}

export type AwsRdsAction = 'snapshot' | 'stop' | 'start' | 'adopt'

export interface AwsRdsCreate {
  id: string
  engine: string
  class: string
  master_username: string
  allocated_gb: number
  multi_az: boolean
  confirm: boolean
}

const enc = encodeURIComponent
const base = (id: string) => `/api/v1/cloud-accounts/${enc(id)}/aws/rds`
const key = (id: string) => [...cloudAccountKeys.all, 'aws', id, 'rds'] as const

export function useAwsRds(id: string) {
  return useQuery({
    queryKey: key(id),
    retry: false,
    queryFn: async () =>
      (
        await apiRequest<{ instances: AwsRdsInstance[] | null }>(
          `${base(id)}/instances`,
          { fallbackMessage: 'load databases failed' },
        )
      ).instances ?? [],
  })
}

export function useAwsRdsEstimate(
  id: string,
  engine: string,
  cls: string,
  multiAz: boolean,
) {
  return useQuery({
    queryKey: [...key(id), 'estimate', engine, cls, multiAz],
    enabled: engine !== '' && cls !== '',
    retry: false,
    queryFn: () =>
      apiRequest<AwsRdsEstimate>(
        `${base(id)}/estimate?engine=${enc(engine)}&class=${enc(cls)}&multi_az=${String(multiAz)}`,
        { fallbackMessage: 'estimate failed' },
      ),
  })
}

export function useAwsRdsAction(id: string) {
  const qc = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (v: { rid: string; action: AwsRdsAction }) =>
      apiRequest<unknown>(`${base(id)}/instances/${enc(v.rid)}/actions`, {
        method: 'POST',
        body: { action: v.action },
        fallbackMessage: 'database action failed',
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: key(id) }),
  })
}

export function useAwsRdsCreate(id: string) {
  const qc = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (body: AwsRdsCreate) =>
      apiRequest<unknown>(`${base(id)}/instances`, {
        method: 'POST',
        body,
        fallbackMessage: 'create database failed',
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: key(id) }),
  })
}
