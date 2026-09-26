// POST /api/v1/imports/platform/discover and /apply
// (internal/api/platform_import.go). The source token travels in the
// request body only and is never cached: these are mutations, so nothing
// lands in the query cache.

import { useMutation } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type ImportPlatform = 'coolify' | 'dokploy' | 'caprover'

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
    mutationFn: (req: PlatformImportRequest) => postImport('discover', req),
  })
}

export function useApplyPlatformImport() {
  return useMutation({
    mutationFn: (req: PlatformImportRequest) => postImport('apply', req),
  })
}
