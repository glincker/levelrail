import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

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

async function json(res: Response, what: string): Promise<AppExpiry> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppExpiry
}

export function useAppExpiry(app: string) {
  return useQuery({
    queryKey: key(app),
    queryFn: async ({ signal }) =>
      json(await fetch(path(app), { signal }), 'fetch app expiry'),
  })
}

export function useSetAppExpiry(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: SetAppExpiryInput) =>
      json(
        await fetch(path(app), {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(input),
        }),
        'set app expiry',
      ),
    onSuccess: (data) => qc.setQueryData(key(app), data),
  })
}

export function useClearAppExpiry(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () =>
      json(await fetch(path(app), { method: 'DELETE' }), 'clear app expiry'),
    onSuccess: (data) => qc.setQueryData(key(app), data),
  })
}
