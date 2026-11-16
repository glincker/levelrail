import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest } from '../lib/apiRequest'

// Matches internal/api's appExpiryResource.
export interface AppExpiry {
  app: string
  expiry?: {
    expires_at: string
    delete_data: boolean
    set_by?: string
    stopped_at?: string
    kept_reason?: string
  }
  state: 'none' | 'scheduled' | 'noticed' | 'stopped' | 'kept'
  delete_after?: string
  delete_enabled: boolean
  grace_hours: number
  notice_hours: number
  max_horizon_days: number
}

export interface SetAppExpiryInput {
  expires_at: string
  delete_data: boolean
  confirm: string
}

const key = (app: string) => ['app-expiry', app] as const
const path = (app: string) => `/api/v1/apps/${encodeURIComponent(app)}/expiry`

function call(
  app: string,
  what: string,
  init?: { method?: string; body?: unknown; signal?: AbortSignal },
): Promise<AppExpiry> {
  return apiRequest<AppExpiry>(path(app), {
    ...init,
    fallbackMessage: `${what} failed`,
  })
}

export function useAppExpiry(app: string) {
  return useQuery({
    queryKey: key(app),
    queryFn: ({ signal }) => call(app, 'fetch app expiry', { signal }),
  })
}

export function useSetAppExpiry(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: SetAppExpiryInput) =>
      call(app, 'set app expiry', { method: 'PUT', body: input }),
    onSuccess: (data) => qc.setQueryData(key(app), data),
  })
}

export function useClearAppExpiry(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => call(app, 'clear app expiry', { method: 'DELETE' }),
    onSuccess: (data) => qc.setQueryData(key(app), data),
  })
}
