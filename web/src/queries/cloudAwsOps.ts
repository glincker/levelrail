import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '../lib/apiRequest'
import { cloudAccountKeys } from './cloudAccounts'

export interface AwsSnapshot {
  id: string
  volume_id: string
  state: string
  size_gb: number
  started_at: string
  description: string
  managed: boolean
  expires_at: string
}

export interface AwsVolume {
  id: string
  size_gb: number
  type: string
  state: string
  az: string
  encrypted: boolean
  instance_id: string
  managed: boolean
}

export interface AwsSnapshotCreate {
  region?: string
  instance_id?: string
  volume_id?: string
  exclude_boot?: boolean
  description?: string
}

export interface AwsMetricSeries {
  name: string
  points: { t: string; v: number }[]
}

export interface AwsMetrics {
  region: string
  instance_id: string
  period: number
  series: AwsMetricSeries[]
  cached: boolean
}

export interface AwsLogGroup {
  name: string
  retention_days: number
  stored_bytes: number
}

export interface AwsLogEvent {
  ts: string
  stream: string
  message: string
}

export interface AwsLogEvents {
  region: string
  events: AwsLogEvent[]
  truncated: boolean
}

export type AwsIdleKind =
  | 'ec2_low_cpu'
  | 'ebs_unattached'
  | 'eip_unassociated'
  | 'alb_no_requests'
  | 'rds_stopped'
  | 'rds_cluster_restart'

export type AwsIdleAction =
  'stop' | 'snapshot_delete' | 'release' | 'delete' | 'restop'

export interface AwsIdleSuggestion {
  kind: AwsIdleKind
  id: string
  detail: string
  action: AwsIdleAction
  est_monthly_usd: number
}

const enc = encodeURIComponent
const base = (id: string) => `/api/v1/cloud-accounts/${enc(id)}/aws`
const key = (id: string, part: string) =>
  [...cloudAccountKeys.all, 'aws', id, part] as const

export function useAwsSnapshots(id: string) {
  return useQuery({
    queryKey: key(id, 'ebs-snapshots'),
    retry: false,
    queryFn: async () =>
      (
        await apiRequest<{ snapshots: AwsSnapshot[] | null }>(
          `${base(id)}/ebs/snapshots`,
          { fallbackMessage: 'load snapshots failed' },
        )
      ).snapshots ?? [],
  })
}

export function useAwsVolumes(id: string) {
  return useQuery({
    queryKey: key(id, 'ebs-volumes'),
    retry: false,
    queryFn: async () =>
      (
        await apiRequest<{ volumes: AwsVolume[] | null }>(
          `${base(id)}/ebs/volumes`,
          { fallbackMessage: 'load volumes failed' },
        )
      ).volumes ?? [],
  })
}

export function useAwsSnapshotCreate(id: string) {
  const qc = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (body: AwsSnapshotCreate) =>
      apiRequest<unknown>(`${base(id)}/ebs/snapshots`, {
        method: 'POST',
        body,
        fallbackMessage: 'create snapshot failed',
      }),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: key(id, 'ebs-snapshots') }),
  })
}

export function useAwsSnapshotDelete(id: string) {
  const qc = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (sid: string) =>
      apiRequest<unknown>(`${base(id)}/ebs/snapshots/${enc(sid)}`, {
        method: 'DELETE',
        fallbackMessage: 'delete snapshot failed',
      }),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: key(id, 'ebs-snapshots') }),
  })
}

export function useAwsMetrics(
  id: string,
  instanceId: string,
  names: string,
  hours: number,
) {
  return useQuery({
    queryKey: key(id, `metrics-${instanceId}-${names}-${String(hours)}`),
    enabled: instanceId !== '',
    retry: false,
    queryFn: () =>
      apiRequest<AwsMetrics>(
        `${base(id)}/cloudwatch/metrics?instance_id=${enc(instanceId)}&names=${enc(names)}&hours=${String(hours)}`,
        { fallbackMessage: 'load metrics failed' },
      ),
  })
}

export function useAwsLogGroups(id: string, prefix: string, limit: number) {
  return useQuery({
    queryKey: key(id, `log-groups-${prefix}-${String(limit)}`),
    retry: false,
    queryFn: async () =>
      (
        await apiRequest<{ groups: AwsLogGroup[] | null }>(
          `${base(id)}/cloudwatch/log-groups?prefix=${enc(prefix)}&limit=${String(limit)}`,
          { fallbackMessage: 'load log groups failed' },
        )
      ).groups ?? [],
  })
}

export function useAwsLogEvents(
  id: string,
  group: string,
  filter: string,
  hours: number,
  limit: number,
) {
  return useQuery({
    queryKey: key(id, `log-events-${group}-${filter}-${String(hours)}`),
    enabled: group !== '',
    retry: false,
    queryFn: () =>
      apiRequest<AwsLogEvents>(
        `${base(id)}/cloudwatch/log-events?group=${enc(group)}&filter=${enc(filter)}&hours=${String(hours)}&limit=${String(limit)}`,
        { fallbackMessage: 'load log events failed' },
      ),
  })
}

export function useAwsIdle(id: string) {
  return useQuery({
    queryKey: key(id, 'idle'),
    retry: false,
    queryFn: async () =>
      (
        await apiRequest<{ suggestions: AwsIdleSuggestion[] | null }>(
          `${base(id)}/idle`,
          { fallbackMessage: 'load idle suggestions failed' },
        )
      ).suggestions ?? [],
  })
}
