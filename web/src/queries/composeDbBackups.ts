// Queries and mutations for database services inside compose apps:
// /api/v1/apps/{name}/compose-db-services (internal/api/compose_db_backups.go).

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  ComposeDbBackupRun,
  ComposeDbService,
  SetComposeDbScheduleRequest,
} from '../types/composeDbBackups'

const RUNNING_POLL_INTERVAL_MS = 3_000

export const composeDbKeys = {
  services: (app: string) => ['apps', app, 'compose-db-services'] as const,
  runs: (app: string, service: string) =>
    ['apps', app, 'compose-db-services', service, 'backups'] as const,
}

function servicePath(app: string, service: string): string {
  return `/api/v1/apps/${encodeURIComponent(app)}/compose-db-services/${encodeURIComponent(service)}`
}

async function ensureOk(res: Response, what: string): Promise<void> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
}

export async function fetchComposeDbServices(
  app: string,
): Promise<ComposeDbService[]> {
  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(app)}/compose-db-services`,
  )
  await ensureOk(res, 'fetch compose db services')
  return (await res.json()) as ComposeDbService[]
}

export function useComposeDbServices(app: string) {
  return useQuery({
    queryKey: composeDbKeys.services(app),
    queryFn: () => fetchComposeDbServices(app),
  })
}

export async function fetchComposeDbRuns(
  app: string,
  service: string,
): Promise<ComposeDbBackupRun[]> {
  const res = await fetch(`${servicePath(app, service)}/backups`)
  await ensureOk(res, 'fetch compose db backups')
  return (await res.json()) as ComposeDbBackupRun[]
}

// Polls only while the newest run is still running.
export function useComposeDbRuns(
  app: string,
  service: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: composeDbKeys.runs(app, service),
    queryFn: () => fetchComposeDbRuns(app, service),
    enabled,
    refetchInterval: (query) =>
      query.state.data?.[0]?.status === 'running'
        ? RUNNING_POLL_INTERVAL_MS
        : false,
  })
}

export function useSetComposeDbSchedule(app: string, service: string) {
  const qc = useQueryClient()
  return useMutation<unknown, ApiError, SetComposeDbScheduleRequest>({
    mutationFn: async (req) => {
      const res = await fetch(`${servicePath(app, service)}/backup-schedule`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(req),
      })
      await ensureOk(res, 'set compose db backup schedule')
    },
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: composeDbKeys.services(app) }),
  })
}

export function useClearComposeDbSchedule(app: string, service: string) {
  const qc = useQueryClient()
  return useMutation<void, ApiError, void>({
    mutationFn: async () => {
      const res = await fetch(`${servicePath(app, service)}/backup-schedule`, {
        method: 'DELETE',
      })
      await ensureOk(res, 'remove compose db backup schedule')
    },
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: composeDbKeys.services(app) }),
  })
}

export function useTriggerComposeDbBackup(app: string, service: string) {
  const qc = useQueryClient()
  return useMutation<unknown, ApiError, { target_id: string }>({
    mutationFn: async (req) => {
      const res = await fetch(`${servicePath(app, service)}/backups`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(req),
      })
      await ensureOk(res, 'start compose db backup')
    },
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: composeDbKeys.runs(app, service) }),
  })
}
