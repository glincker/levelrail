import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '../lib/apiRequest'
import { cloudAccountKeys } from './cloudAccounts'

export interface CloudInstanceEvent {
  time: string
  kind: string
  status: string
  message?: string
}

export interface CloudInstanceEvents {
  state: 'ok' | 'unsupported'
  since: string
  events: CloudInstanceEvent[]
}

export function useCloudInstanceEvents(
  id: string,
  iid: string | null,
  enabled: boolean,
) {
  return useQuery({
    queryKey: [...cloudAccountKeys.instance(id, iid ?? ''), 'events'],
    enabled: enabled && iid !== null,
    retry: false,
    queryFn: () =>
      apiRequest<CloudInstanceEvents>(
        `/api/v1/cloud-accounts/${encodeURIComponent(id)}/instances/${encodeURIComponent(iid ?? '')}/events`,
        { fallbackMessage: 'load instance events failed' },
      ),
  })
}
