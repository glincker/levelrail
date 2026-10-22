import { ApiError, readErrorMessage } from '../lib/apiError'
import type { ImportAnalysis } from './importAnalyze'

// POST /api/v1/git-sources/preview (internal/api/repo_preview.go): a
// read-only repository card. Host failures come back as a `kind`, not an error.

export type PreviewKind =
  | 'public'
  | 'private_needs_account'
  | 'not_found'
  | 'rate_limited'
  | 'unsupported_host'
  | 'invalid_url'

export interface PreviewLanguage {
  name: string
  percent: number
}

export interface PreviewCommit {
  sha: string
  message: string
  author: string
  at: string
}

export interface RepoPreview {
  kind: PreviewKind
  host?: string
  repo_url?: string
  owner?: string
  name?: string
  description?: string
  default_branch?: string
  branches?: string[]
  visibility?: 'public' | 'private'
  stars?: number
  forks?: number
  languages?: PreviewLanguage[]
  last_commit?: PreviewCommit
  license?: string
  topics?: string[]
  readme_excerpt?: string
  size_kb?: number
  analysis?: ImportAnalysis
}

export interface RepoPreviewRequest {
  repo_url: string
  ref?: string
  account_id?: string
  token?: string
}

export const repoPreviewKeys = {
  one: (url: string, ref: string, accountId: string, tokenSet: boolean) =>
    ['repo-preview', url, ref, accountId, tokenSet] as const,
}

export async function previewRepo(
  req: RepoPreviewRequest,
  signal?: AbortSignal,
): Promise<RepoPreview> {
  const res = await fetch('/api/v1/git-sources/preview', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
    signal,
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `preview repository failed: ${res.status}`),
    )
  }
  return (await res.json()) as RepoPreview
}
