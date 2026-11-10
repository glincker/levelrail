// POST /api/v1/imports/platform/discover and /apply
// (internal/api/platform_import.go). The source token travels in the
// request body only and is never cached: these are mutations, so nothing
// lands in the query cache.

import { useMutation } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type ImportPlatform =
  'coolify' | 'dokploy' | 'caprover' | 'heroku' | 'vercel'

export type ImportItemStatus =
  | 'mapped'
  | 'needs-attention'
  | 'unsupported'
  | 'already-imported'
  | 'skipped'
  | 'created'
  | 'failed'

export interface PlatformImportRequest {
  platform: ImportPlatform
  url: string
  token: string
  insecure_tls?: boolean
  allow_private?: boolean
  allow_loopback?: boolean
  only?: string[]
  collision?: 'suffix' | 'skip'
  db_versions?: Record<string, string>
  team_id?: string
  include_credentials?: boolean
  enable_backup_schedules?: boolean
  enable_imported_channels?: boolean
  extras_only?: boolean
  import_extras?: boolean
  journal?: ImportJournalRecord[]
}

export interface ImportBackupTarget {
  source_name: string
  name: string
  provider: string
  endpoint?: string
  region?: string
  bucket: string
  id?: string
  state: string
  detail?: string
  has_keys: boolean
}

export interface ImportBackupSchedule {
  database: string
  target: string
  cron: string
  retain?: number
  paused: boolean
  state: string
  detail?: string
}

export interface ImportChannel {
  source_name: string
  name: string
  kind: string
  server?: string
  enabled: boolean
  id?: string
  state: string
  detail?: string
  has_secret: boolean
}

export interface ImportUnsupportedChannel {
  source_name: string
  source_kind: string
  reason: string
}

// A journal record names what an apply created, by id and fingerprint only.
export interface ImportJournalRecord {
  kind: string
  name: string
  id?: string
  fingerprint?: string
  cron?: string
  pause_failed?: boolean
}

export interface ImportUndoItem {
  kind: string
  name: string
  id?: string
  action: 'removed' | 'skipped' | 'failed'
  reason?: string
}

export interface PlatformImportItem {
  kind: string
  source_id: string
  source_name: string
  target?: string
  status: ImportItemStatus
  reasons?: string[]
  manual?: string[]
}

export interface PlatformImportReport {
  platform: ImportPlatform
  items: PlatformImportItem[]
  counts: Record<string, number>
  notes?: string[]
  backups?: { targets: ImportBackupTarget[]; schedules: ImportBackupSchedule[] }
  channels?: {
    channels: ImportChannel[]
    unsupported?: ImportUnsupportedChannel[]
  }
  journal?: ImportJournalRecord[]
}

async function postImport(
  action: 'discover' | 'apply',
  req: PlatformImportRequest,
): Promise<PlatformImportReport> {
  const res = await fetch(`/api/v1/imports/platform/${action}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `platform import ${action} failed`),
    )
  }
  return (await res.json()) as PlatformImportReport
}

export function useDiscoverPlatformImport() {
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (req: PlatformImportRequest) => postImport('discover', req),
  })
}

export function useApplyPlatformImport() {
  return useMutation({
    meta: { errorToast: false },
    mutationFn: (req: PlatformImportRequest) => postImport('apply', req),
  })
}

async function postUndo(
  journal: ImportJournalRecord[],
): Promise<ImportUndoItem[]> {
  const res = await fetch('/api/v1/imports/platform/undo', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ journal }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, 'platform import undo failed'),
    )
  }
  return ((await res.json()) as { items: ImportUndoItem[] }).items
}

export function useUndoPlatformImport() {
  return useMutation({
    meta: { errorToast: false },
    mutationFn: postUndo,
  })
}
