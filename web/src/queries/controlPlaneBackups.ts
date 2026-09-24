import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface ControlPlaneBackup {
  name: string
  size_bytes: number
  created_at: string
  sha256: string
}

export const controlPlaneBackupKeys = {
  all: ['control-plane-backups'] as const,
  list: () => [...controlPlaneBackupKeys.all, 'list'] as const,
}

const BASE = '/api/v1/system/backups'

export function controlPlaneBackupDownloadUrl(name: string): string {
  return `${BASE}/${encodeURIComponent(name)}/download`
}

export async function fetchControlPlaneBackups(): Promise<
  ControlPlaneBackup[]
> {
  const res = await fetch(BASE)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `list backups failed: ${res.status}`),
    )
  }
  return (await res.json()) as ControlPlaneBackup[]
}

export async function createControlPlaneBackup(): Promise<ControlPlaneBackup> {
  const res = await fetch(BASE, { method: 'POST' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create backup failed: ${res.status}`),
    )
  }
  return (await res.json()) as ControlPlaneBackup
}

export async function deleteControlPlaneBackup(name: string): Promise<void> {
  const res = await fetch(`${BASE}/${encodeURIComponent(name)}`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `delete backup failed: ${res.status}`),
    )
  }
}

export function useControlPlaneBackups() {
  return useQuery({
    queryKey: controlPlaneBackupKeys.list(),
    queryFn: fetchControlPlaneBackups,
    retry: false,
  })
}

export function useCreateControlPlaneBackup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: createControlPlaneBackup,
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: controlPlaneBackupKeys.all }),
  })
}

export function useDeleteControlPlaneBackup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: deleteControlPlaneBackup,
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: controlPlaneBackupKeys.all }),
  })
}
