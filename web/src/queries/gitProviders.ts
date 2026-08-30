// Query-key factory and fetcher for the aggregated git provider
// capability summary (internal/api/git_providers.go).
//
// This replaces three separate useGitHubAppStatus/useGitLabAppStatus/
// useBitbucketAppStatus suspense queries in GitRepoSourcePicker.tsx: all
// three of those hit AbilityRoot endpoints, so a non-root deploy-scoped
// user opening the app creation wizard used to throw into an error
// boundary before this endpoint existed. This one call sits at
// AbilityReadSensitive instead, matching what GET .../repos already
// discloses at that same tier for each provider individually.

import { queryOptions, useSuspenseQuery } from '@tanstack/react-query'
import type { GitProviderStatus } from '../types/gitProviders'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const gitProviderKeys = {
  all: ['git-providers'] as const,
}

export async function fetchGitProviders(): Promise<GitProviderStatus[]> {
  const res = await fetch('/api/v1/git-providers')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch git providers failed: ${res.status}`),
    )
  }
  return (await res.json()) as GitProviderStatus[]
}

export function gitProvidersQueryOptions() {
  return queryOptions({
    queryKey: gitProviderKeys.all,
    queryFn: fetchGitProviders,
  })
}

export function useGitProviders() {
  return useSuspenseQuery(gitProvidersQueryOptions())
}
