import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '../lib/apiRequest'
import { cloudAccountKeys } from './cloudAccounts'

export interface AwsLightsailInstance {
  name: string
  state: string
  bundle_id: string
  blueprint_id: string
  public_ip?: string
  zone: string
  managed: boolean
}

const enc = encodeURIComponent
const base = (id: string) => `/api/v1/cloud-accounts/${enc(id)}/aws/lightsail`
const key = (id: string) =>
  [...cloudAccountKeys.all, 'aws', id, 'lightsail'] as const

export function useAwsLightsailInstances(id: string) {
  return useQuery({
    queryKey: [...key(id), 'instances'],
    retry: false,
    queryFn: async () =>
      (
        await apiRequest<{ instances: AwsLightsailInstance[] | null }>(
          `${base(id)}/instances`,
          { fallbackMessage: 'load servers failed' },
        )
      ).instances ?? [],
  })
}

export type AwsLightsailAction =
  | { kind: 'start' | 'stop' | 'reboot'; name: string }
  | { kind: 'snapshot'; name: string }
  | { kind: 'delete'; name: string }

export function useAwsLightsailAction(id: string) {
  const qc = useQueryClient()
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (v: AwsLightsailAction) => {
      if (v.kind === 'snapshot') {
        return apiRequest<unknown>(`${base(id)}/snapshots`, {
          method: 'POST',
          body: { instance: v.name },
          fallbackMessage: 'server snapshot failed',
        })
      }
      if (v.kind === 'delete') {
        return apiRequest<unknown>(
          `${base(id)}/instances/${enc(v.name)}?confirm=${enc(v.name)}`,
          { method: 'DELETE', fallbackMessage: 'server delete failed' },
        )
      }
      return apiRequest<unknown>(
        `${base(id)}/instances/${enc(v.name)}/${v.kind}`,
        { method: 'POST', body: {}, fallbackMessage: 'server change failed' },
      )
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: key(id) }),
  })
}
