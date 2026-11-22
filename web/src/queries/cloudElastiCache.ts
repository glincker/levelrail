import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '../lib/apiRequest'
import { cloudAccountKeys } from './cloudAccounts'

export interface AwsCacheGroup {
  id: string
  status: string
  node_type: string
  primary_endpoint: string
  port: number
  members: string[]
  managed: boolean
}

export interface AwsCacheCluster {
  id: string
  engine: string
  status: string
  node_type: string
  replication_group_id?: string
  managed: boolean
}

const enc = encodeURIComponent
const base = (id: string) => `/api/v1/cloud-accounts/${enc(id)}/aws/elasticache`
const key = (id: string) =>
  [...cloudAccountKeys.all, 'aws', id, 'elasticache'] as const

export function useAwsCacheGroups(id: string) {
  return useQuery({
    queryKey: [...key(id), 'groups'],
    retry: false,
    queryFn: async () =>
      (
        await apiRequest<{ replication_groups: AwsCacheGroup[] | null }>(
          `${base(id)}/replication-groups`,
          { fallbackMessage: 'load caches failed' },
        )
      ).replication_groups ?? [],
  })
}

export function useAwsCacheClusters(id: string) {
  return useQuery({
    queryKey: [...key(id), 'clusters'],
    retry: false,
    queryFn: async () =>
      (
        await apiRequest<{ clusters: AwsCacheCluster[] | null }>(
          `${base(id)}/clusters`,
          { fallbackMessage: 'load cache clusters failed' },
        )
      ).clusters ?? [],
  })
}

export type AwsCacheAction =
  | { kind: 'snapshot'; id: string }
  | { kind: 'reboot'; id: string }
  | { kind: 'delete'; id: string; finalSnapshot: string }

export function useAwsCacheAction(id: string) {
  const qc = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (v: AwsCacheAction) => {
      if (v.kind === 'snapshot') {
        return apiRequest<unknown>(`${base(id)}/snapshots`, {
          method: 'POST',
          body: { replication_group: v.id },
          fallbackMessage: 'cache snapshot failed',
        })
      }
      if (v.kind === 'reboot') {
        return apiRequest<unknown>(`${base(id)}/clusters/${enc(v.id)}/reboot`, {
          method: 'POST',
          body: {},
          fallbackMessage: 'cache reboot failed',
        })
      }
      const q = new URLSearchParams({ confirm: v.id })
      if (v.finalSnapshot !== '') q.set('final_snapshot', v.finalSnapshot)
      return apiRequest<unknown>(
        `${base(id)}/replication-groups/${enc(v.id)}?${q.toString()}`,
        { method: 'DELETE', fallbackMessage: 'cache delete failed' },
      )
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: key(id) }),
  })
}
