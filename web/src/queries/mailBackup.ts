// Mail backup and storage APIs (internal/api/mail_backup.go). The S3 secret is write-only.

import { useMutation, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { EnvApiError } from './envProtection'
import { useInvalidateMail } from './mailMailboxes'

export interface MailBackupJob {
  id: string
  kind: 'backup' | 'restore'
  state: 'running' | 'succeeded' | 'failed'
  step: string
  backup_id?: string
  started_at: string
  finished_at?: string
  error?: string
}

export interface MailBackupItem {
  id: string
  created_at: string
  stored_bytes: number
  data_bytes: number
  files: number
  mox_version: string
  verified: boolean
  where: string
}

export interface MailBackupList {
  app: string
  encrypted: boolean
  folder: string
  cap_bytes: number
  used_bytes: number
  backups: MailBackupItem[]
  job?: MailBackupJob
}

export interface MailRestoreResult {
  app: string
  dry_run: boolean
  backup_id: string
  mox_version?: string
  files?: number
  bytes?: number
  job?: MailBackupJob
}

export interface MailStorageStep {
  name: string
  ok: boolean
  error?: string
}

export interface MailStorage {
  app: string
  kind: 'local' | 's3'
  folder: string
  cap_bytes: number
  bucket?: {
    endpoint: string
    region: string
    bucket: string
    path_style: boolean
  }
  steps: MailStorageStep[]
  ok: boolean
  saved: boolean
  bucket_uploads: boolean
}

export interface MailStorageInput {
  app: string
  kind: 'local' | 's3'
  endpoint: string
  region: string
  bucket: string
  access_key_id: string
  secret_access_key: string
  path_style: boolean
}

// A failed probe answers 422 with the per-step result; callers show it.
export class MailStorageProbeError extends Error {
  readonly result: MailStorage
  constructor(result: MailStorage) {
    super('storage probe failed')
    this.result = result
  }
}

const JOB_POLL_MS = Number(import.meta.env.VITE_MAIL_JOB_POLL_MS) || 2000

export const mailBackupKeys = {
  backups: (app: string) => ['mail', 'backups', app] as const,
  storage: (app: string) => ['mail', 'storage', app] as const,
}

export const mailRestoreStepUpScope = (app: string) =>
  ({ action: 'manage', app: `mail-restore:${app}` }) as const

async function send<T>(
  method: string,
  path: string,
  body: unknown,
  okStatuses: number[] = [],
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    cache: 'no-store',
  })
  if (!res.ok && !okStatuses.includes(res.status)) {
    const err = (await res.json().catch(() => null)) as {
      error?: string
      code?: string
      steps?: unknown
    } | null
    if (res.status === 422 && err && Array.isArray(err.steps)) {
      throw new MailStorageProbeError(err as unknown as MailStorage)
    }
    // EnvApiError carries step_up_required so useStepUpGate can catch it.
    throw new EnvApiError(
      res.status,
      err?.error ?? `mail request failed: ${res.status}`,
      err?.code ?? '',
    )
  }
  return (await res.json()) as T
}

async function read<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `mail request failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

const enc = encodeURIComponent

// Polls every 2s while a job runs so the step shows as progress.
export const useMailBackups = (app: string | undefined) =>
  useQuery({
    queryKey: mailBackupKeys.backups(app ?? ''),
    queryFn: () =>
      read<MailBackupList>(`/api/v1/mail/backups?app=${enc(app ?? '')}`),
    enabled: !!app,
    retry: false,
    refetchInterval: (q) =>
      q.state.data?.job?.state === 'running' ? JOB_POLL_MS : false,
  })

export const useMailStorage = (app: string | undefined) =>
  useQuery({
    queryKey: mailBackupKeys.storage(app ?? ''),
    queryFn: () =>
      read<MailStorage>(`/api/v1/mail/storage?app=${enc(app ?? '')}`),
    enabled: !!app,
    retry: false,
  })

export const runMailBackup = (app: string) =>
  send<MailBackupJob>('POST', '/api/v1/mail/backups', { app })

export const restoreDryRun = (app: string, id: string) =>
  send<MailRestoreResult>('POST', `/api/v1/mail/backups/${enc(id)}/restore`, {
    app,
  })

// Step-up is a server session fact: a 403 step_up_required is retried after the dialog.
export const restoreMailBackup = (app: string, id: string) =>
  send<MailRestoreResult>('POST', `/api/v1/mail/backups/${enc(id)}/restore`, {
    app,
    confirm: id,
  })

export const saveMailStorage = (input: MailStorageInput) =>
  send<MailStorage>('PUT', '/api/v1/mail/storage', input)

export const testMailStorage = (app: string) =>
  send<MailStorage>('POST', '/api/v1/mail/storage/test', { app })

/** Starts a backup and refreshes mail data; the line and the palette share it. */
export function useRunMailBackup(app: string | undefined) {
  const invalidate = useInvalidateMail()
  return useMutation({
    mutationFn: () => runMailBackup(app ?? ''),
    onSuccess: () => invalidate(),
  })
}
