// Fetchers and hooks for the provider-neutral repository listing
// (GET /api/v1/git/repos): searched, filtered and paged on the server from
// a per-connection cache, so a huge account never loads in one response.

import {
  keepPreviousData,
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  GitRepoListParams,
  GitRepoListResponse,
  GitRepoProvider,
} from '../types/gitRepos'

export const GIT_REPO_PAGE_SIZE = 100

export const gitRepoKeys = {
  all: ['git-repos'] as const,
  provider: (provider: GitRepoProvider | '') =>
    [...gitRepoKeys.all, provider || 'all'] as const,
  list: (provider: GitRepoProvider | '', params: GitRepoListParams) =>
    [...gitRepoKeys.provider(provider), params] as const,
}

// A 429 from the control plane: the provider is rate limiting and there is no
// saved list to show yet. nextCheckAt tells the picker when to try again.
export class GitRepoRateLimitError extends ApiError {
  readonly nextCheckAt: string

  constructor(message: string, nextCheckAt: string) {
    super(429, message)
    this.name = 'GitRepoRateLimitError'
    this.nextCheckAt = nextCheckAt
  }
}

export function gitRepoListUrl(
  provider: GitRepoProvider | '',
  params: GitRepoListParams,
): string {
  const q = new URLSearchParams()
  if (provider) q.set('provider', provider)
  if (params.connection) q.set('connection', params.connection)
  q.set('limit', String(params.limit ?? GIT_REPO_PAGE_SIZE))
  if (params.search) q.set('search', params.search)
  if (params.owner) q.set('owner', params.owner)
  if (params.archived && params.archived !== 'include') {
    q.set('archived', params.archived)
  }
  if (params.forks && params.forks !== 'include') q.set('forks', params.forks)
  if (params.offset) q.set('offset', String(params.offset))
  if (params.refresh) q.set('refresh', '1')
  return `/api/v1/git/repos?${q.toString()}`
}

export async function fetchGitRepoList(
  provider: GitRepoProvider | '',
  params: GitRepoListParams,
): Promise<GitRepoListResponse> {
  const res = await fetch(gitRepoListUrl(provider, params))
  if (res.status === 429) {
    const body = (await res.json().catch(() => null)) as {
      error?: string
      next_check_at?: string
    } | null
    throw new GitRepoRateLimitError(
      body?.error ?? `${provider || 'the git provider'} is rate limiting`,
      body?.next_check_at ?? '',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch ${provider || 'git'} repositories failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as GitRepoListResponse
}

export function useGitRepoList(
  provider: GitRepoProvider | '',
  params: GitRepoListParams,
  enabled: boolean,
) {
  return useInfiniteQuery({
    queryKey: gitRepoKeys.list(provider, params),
    queryFn: ({ pageParam }) =>
      fetchGitRepoList(provider, { ...params, offset: pageParam }),
    initialPageParam: 0,
    getNextPageParam: (last) => last.next_offset ?? undefined,
    placeholderData: keepPreviousData,
    enabled,
  })
}

// Re-lists the provider now instead of serving the cache, then reloads every
// open list for that provider.
export function useRefreshGitRepos(provider: GitRepoProvider | '') {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => fetchGitRepoList(provider, { refresh: true }),
    onSettled: () =>
      queryClient.invalidateQueries({
        queryKey: gitRepoKeys.provider(provider),
      }),
  })
}
