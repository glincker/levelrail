// Query-key factory and fetcher for POST /api/v1/git/branches
// (internal/api/git_branches.go's handleListGitBranches): every branch
// name a public git remote advertises, for
// CreateAppFromGitFields.tsx's (via GitBuildSourceFields.tsx) branch
// picker. Kept in its own module, the same "distinct resource, own
// file" reasoning queries/images.ts's own doc comment gives for a
// different app-adjacent-but-not-quite-app-shaped resource.
//
// Unlike useImageTagsOptional (queries/images.ts), a failed lookup here
// is surfaced as a real, distinct error state rather than silently
// degrading to "nothing to suggest": an unreachable or private repo_url
// is very often the caller's own actionable mistake (a typo, a private
// repo they meant to make public first), not a background nicety a
// dashboard failure should paper over.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const gitBranchesKeys = {
  list: (repoUrl: string) => ['git-branches', repoUrl] as const,
}

interface GitBranchesResponse {
  branches: string[] | null
}

export async function fetchGitBranches(repoUrl: string): Promise<string[]> {
  const res = await fetch('/api/v1/git/branches', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ repo_url: repoUrl }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `list branches failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as GitBranchesResponse | null
  return body?.branches ?? []
}

export function gitBranchesQueryOptions(repoUrl: string) {
  return queryOptions({
    queryKey: gitBranchesKeys.list(repoUrl),
    queryFn: () => fetchGitBranches(repoUrl),
  })
}

// repoUrl is null until the caller explicitly requests a load (a "Load
// branches" button click sets it, see GitBuildSourceFields.tsx), not
// fired automatically on every keystroke: this endpoint makes a real
// network call against whatever URL it's given, and a URL still being
// typed is very often not yet a valid one to call it with.
export function useGitBranches(repoUrl: string | null) {
  return useQuery({
    ...gitBranchesQueryOptions(repoUrl ?? ''),
    enabled: repoUrl !== null && repoUrl !== '',
    retry: false,
  })
}
