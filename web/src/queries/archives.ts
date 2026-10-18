import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// Matches internal/archive.Bundle.
export interface ArchiveBundle {
  id: string
  kind: 'audit' | 'deploys' | 'pipelines'
  created_at: string
  cutoff: string
  rows: Record<string, number>
  total_rows: number
  size_bytes: number
  sha256: string
  verified_at?: string
  verified_ok?: boolean
  restored_at?: string
}

// Matches internal/store.ArchiveSettings.
export interface ArchivePolicy {
  enabled: boolean
  audit_hot_days: number
  deploy_hot_days: number
  pipeline_hot_days: number
  keep_days: number
  last_run_at?: string
  last_run_error?: string
}

export interface ArchiveList {
  bundles: ArchiveBundle[]
  policy: ArchivePolicy
  hot_rows: Record<string, number>
  total_size_bytes: number
}

export interface ArchiveRunResult {
  created: ArchiveBundle[]
  pruned: number
}

export interface ArchiveRestoreResult {
  restored: Record<string, number>
  already_present: number
  skipped_missing_parent: number
}

export interface ArchiveVerifyResult {
  bundle: ArchiveBundle
  ok: boolean
  error?: string
}

export interface ArchiveRows {
  table: string
  total: number
  rows: Record<string, unknown>[]
}

const BASE = '/api/v1/archives'

export const archiveKeys = {
  all: ['archives'] as const,
  list: () => [...archiveKeys.all, 'list'] as const,
  rows: (id: string, table: string, offset: number) =>
    [...archiveKeys.all, 'rows', id, table, offset] as const,
}

export function archiveDownloadUrl(id: string): string {
  return `${BASE}/${encodeURIComponent(id)}/download`
}

async function request<T>(
  path: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(path, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) {
    return undefined as T
  }
  return (await res.json()) as T
}

export function useArchives() {
  return useQuery({
    queryKey: archiveKeys.list(),
    queryFn: () => request<ArchiveList>(BASE, undefined, 'list archives'),
    retry: false,
  })
}

export function useArchiveRows(id: string, table: string, offset: number) {
  return useQuery({
    queryKey: archiveKeys.rows(id, table, offset),
    queryFn: () =>
      request<ArchiveRows>(
        `${BASE}/${encodeURIComponent(id)}/rows?table=${encodeURIComponent(table)}&offset=${offset}&limit=25`,
        undefined,
        'browse archive',
      ),
    enabled: table !== '',
  })
}

function useArchiveMutation<V, T>(fn: (v: V) => Promise<T>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => qc.invalidateQueries({ queryKey: archiveKeys.all }),
  })
}

export function useRunArchive() {
  return useArchiveMutation(() =>
    request<ArchiveRunResult>(
      BASE,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: '{}',
      },
      'archive',
    ),
  )
}

export function useRestoreArchive() {
  return useArchiveMutation((id: string) =>
    request<ArchiveRestoreResult>(
      `${BASE}/${encodeURIComponent(id)}/restore`,
      { method: 'POST' },
      'restore archive',
    ),
  )
}

export function useVerifyArchive() {
  return useArchiveMutation((id: string) =>
    request<ArchiveVerifyResult>(
      `${BASE}/${encodeURIComponent(id)}/verify`,
      { method: 'POST' },
      'verify archive',
    ),
  )
}

export function useDeleteArchive() {
  return useArchiveMutation((id: string) =>
    request<void>(
      `${BASE}/${encodeURIComponent(id)}`,
      { method: 'DELETE' },
      'delete archive',
    ),
  )
}

export function useSaveArchivePolicy() {
  return useArchiveMutation((p: ArchivePolicy) =>
    request<ArchivePolicy>(
      `${BASE}/policy`,
      {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(p),
      },
      'save archive policy',
    ),
  )
}
