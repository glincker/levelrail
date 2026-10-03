// Query-key factory and fetcher for GET /api/v1/changelog
// (internal/api/changelog.go's handleGetChangelog): recent release
// notes parsed from the control plane's own CHANGELOG.md, the
// dashboard's "What's new" panel data source.

import { queryOptions } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface ChangelogEntry {
  version: string
  date: string
  bullets: string[]
}

export interface ChangelogResponse {
  current_version: string
  entries: ChangelogEntry[]
}

export const changelogKeys = {
  all: ['changelog'] as const,
}

export async function fetchChangelog(): Promise<ChangelogResponse> {
  const res = await fetch('/api/v1/changelog')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch changelog failed: ${res.status}`),
    )
  }
  return (await res.json()) as ChangelogResponse
}

export function changelogQueryOptions() {
  return queryOptions({
    queryKey: changelogKeys.all,
    queryFn: fetchChangelog,
    staleTime: 5 * 60_000,
  })
}
