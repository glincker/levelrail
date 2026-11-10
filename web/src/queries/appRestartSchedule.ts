import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Matches internal/api's appRestartScheduleResource.
export interface AppRestartSchedule {
  app: string
  schedule?: {
    schedule: string
    timezone: string
    set_by?: string
    next_run_at: string
    last_run_at?: string
    last_status?: 'started' | 'succeeded' | 'skipped' | 'failed'
    last_reason?: string
    failures: number
  }
  min_interval: string
}

export interface SetAppRestartScheduleInput {
  schedule: string
  timezone: string
}

const key = (app: string) => ['app-restart-schedule', app] as const
const path = (app: string) =>
  `/api/v1/apps/${encodeURIComponent(app)}/restart-schedule`

async function json(res: Response, what: string): Promise<AppRestartSchedule> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppRestartSchedule
}

export function useAppRestartSchedule(app: string) {
  return useQuery({
    queryKey: key(app),
    queryFn: async ({ signal }) =>
      json(await fetch(path(app), { signal }), 'fetch restart schedule'),
  })
}

export function useSetAppRestartSchedule(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async (input: SetAppRestartScheduleInput) =>
      json(
        await fetch(path(app), {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(input),
        }),
        'set restart schedule',
      ),
    onSuccess: (data) => qc.setQueryData(key(app), data),
  })
}

export function useClearAppRestartSchedule(app: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: async () =>
      json(
        await fetch(path(app), { method: 'DELETE' }),
        'clear restart schedule',
      ),
    onSuccess: (data) => qc.setQueryData(key(app), data),
  })
}
