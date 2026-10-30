import { queryOptions } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface RepoPlanWarning {
  code: string
  message: string
}

export interface RepoPlanRow {
  name: string
  package_name: string
  directory: string
  port: number | null
  port_source?: string
  build_kind: 'dockerfile' | 'buildpack' | 'static'
  dockerfile?: string
  watch_paths: string[]
  depends_on: string[]
  warnings: RepoPlanWarning[]
}

export interface RepoPlan {
  repo_url: string
  branch: string
  base_directory?: string
  tool?: string
  package_cap: number
  packages_found: number
  packages: RepoPlanRow[] | null
  warnings: RepoPlanWarning[] | null
  totals: { packages: number; warnings: number; flagged: number }
}

export interface RepoPlanRequest {
  repo_url: string
  branch?: string
  base_directory?: string
  /** One-shot token for a private pasted repo; never part of the cache key. */
  token?: string
}

export const repoPlanKeys = {
  plan: (repoUrl: string, branch: string, dir: string) =>
    ['import-repo-plan', repoUrl, branch, dir] as const,
}

export async function fetchRepoPlan(req: RepoPlanRequest): Promise<RepoPlan> {
  const res = await fetch('/api/v1/imports/repo/plan', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `plan import failed: ${res.status}`),
    )
  }
  return (await res.json()) as RepoPlan
}

export function repoPlanOptions(req: RepoPlanRequest, enabled: boolean) {
  return queryOptions({
    queryKey: repoPlanKeys.plan(
      req.repo_url,
      req.branch ?? '',
      req.base_directory ?? '',
    ),
    queryFn: () => fetchRepoPlan(req),
    enabled,
    staleTime: 60_000,
    retry: false,
  })
}
