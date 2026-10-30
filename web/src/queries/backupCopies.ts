// Fetchers for the second-destination backup copy endpoints (internal/api/backup_copies.go).
import {
  queryOptions,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type { BackupCopy, BackupCopySettings } from '../types/backupCopy'

export const backupCopyKeys = {
  all: ['backup-copies'] as const,
  settings: () => [...backupCopyKeys.all, 'settings'] as const,
  list: () => [...backupCopyKeys.all, 'list'] as const,
}

async function readJSON<T>(res: Response, what: string): Promise<T> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export function backupCopySettingsQueryOptions() {
  return queryOptions({
    queryKey: backupCopyKeys.settings(),
    queryFn: async () =>
      readJSON<BackupCopySettings>(
        await fetch('/api/v1/backups/copy-settings'),
        'fetch backup copy settings',
      ),
  })
}

export function backupCopyListQueryOptions() {
  return queryOptions({
    queryKey: backupCopyKeys.list(),
    queryFn: async () =>
      readJSON<BackupCopy[]>(
        await fetch('/api/v1/backups/copies'),
        'fetch backup copies',
      ),
    refetchInterval: 30_000,
  })
}

export function useSetBackupCopySettings() {
  const queryClient = useQueryClient()
  return useMutation<
    BackupCopySettings,
    ApiError,
    { enabled: boolean; target_id: string }
  >({
    mutationFn: async (body) =>
      readJSON<BackupCopySettings>(
        await fetch('/api/v1/backups/copy-settings', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(body),
        }),
        'update backup copy settings',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: backupCopyKeys.all })
    },
  })
}
