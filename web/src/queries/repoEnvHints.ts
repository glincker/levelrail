import { queryOptions } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

// One env var name a repository documents (POST /api/v1/apps/repo-plan).
// The server never returns values, only names and hints.
export interface RepoEnvHint {
  name: string
  source: string
  required: boolean
  looks_secret: boolean
  has_default_hint: boolean
}

export interface RepoEnvHintRequest {
  repo_url: string
  ref?: string
  base_directory?: string
  /** One-shot token for a private repo; never part of the cache key. */
  token?: string
}

export const repoEnvHintKeys = {
  hints: (repoUrl: string, ref: string, dir: string) =>
    ['repo-env-hints', repoUrl, ref, dir] as const,
}

export async function fetchRepoEnvHints(
  req: RepoEnvHintRequest,
): Promise<RepoEnvHint[]> {
  const res = await fetch('/api/v1/apps/repo-plan', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `repo env scan failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as { env?: RepoEnvHint[] | null }
  return body.env ?? []
}

export function repoEnvHintOptions(req: RepoEnvHintRequest, enabled: boolean) {
  return queryOptions({
    queryKey: repoEnvHintKeys.hints(
      req.repo_url,
      req.ref ?? '',
      req.base_directory ?? '',
    ),
    queryFn: () => fetchRepoEnvHints(req),
    enabled,
    staleTime: 60_000,
    retry: false,
    meta: { errorToast: false },
  })
}
