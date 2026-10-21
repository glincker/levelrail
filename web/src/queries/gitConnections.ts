// Query keys, fetchers and mutations for GitLab, Bitbucket and Gitea
// connections (internal/api/git_connections.go). Authorizing a connection is
// a full-page navigation to its authorize_path, so it is not a fetcher here.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  CreateGitConnectionRequest,
  GitConnectionBranch,
  GitConnectionProvider,
  GitProviderConnection,
  UseRepoAsSourceRequest,
} from '../types/gitConnections'
import type { GitDisconnectResult } from '../types/gitProviders'
import type { GitSourceResource } from '../types/gitSource'
import { gitProviderKeys } from './gitProviders'
import { gitRepoKeys } from './gitRepos'
import { gitSourceKeys } from './gitSources'

export const gitConnectionKeys = {
  all: ['git-connections'] as const,
  list: (provider?: GitConnectionProvider) =>
    [...gitConnectionKeys.all, 'list', provider ?? 'all'] as const,
  branches: (connectionId: string, repo: string) =>
    [...gitConnectionKeys.all, 'branches', connectionId, repo] as const,
}

async function request<T>(
  url: string,
  init: RequestInit | undefined,
  failure: string,
): Promise<T> {
  const res = await fetch(url, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${failure}: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

function json(method: string, body?: unknown): RequestInit {
  return {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  }
}

export function gitConnectionsQueryOptions(provider?: GitConnectionProvider) {
  const url = provider
    ? `/api/v1/git-connections?provider=${provider}`
    : '/api/v1/git-connections'
  return queryOptions({
    queryKey: gitConnectionKeys.list(provider),
    queryFn: () =>
      request<GitProviderConnection[]>(
        url,
        undefined,
        'fetch git connections failed',
      ),
  })
}

export function useGitConnections(provider?: GitConnectionProvider) {
  return useSuspenseQuery(gitConnectionsQueryOptions(provider))
}

function useInvalidateGitConnections() {
  const queryClient = useQueryClient()
  return () => {
    void queryClient.invalidateQueries({ queryKey: gitConnectionKeys.all })
    void queryClient.invalidateQueries({ queryKey: gitProviderKeys.all })
    void queryClient.invalidateQueries({ queryKey: gitRepoKeys.all })
  }
}

export function useCreateGitConnection() {
  const invalidate = useInvalidateGitConnections()
  return useMutation<
    GitProviderConnection,
    ApiError,
    CreateGitConnectionRequest
  >({
    mutationFn: (req) =>
      request(
        '/api/v1/git-connections',
        json('POST', req),
        'add account failed',
      ),
    onSuccess: invalidate,
  })
}

export type GitConnectionTestKind =
  'bad_url' | 'bad_token' | 'tls' | 'unreachable'

export interface GitConnectionTestResult {
  ok: boolean
  kind: GitConnectionTestKind | 'ok'
  message: string
  fix?: string
  account?: string
}

// Checks an instance URL and optional token; nothing is saved.
export function useTestGitConnection() {
  return useMutation<
    GitConnectionTestResult,
    ApiError,
    { provider: GitConnectionProvider; instance_url?: string; token?: string }
  >({
    mutationFn: (req) =>
      request(
        '/api/v1/git-connections/test',
        json('POST', req),
        'test connection failed',
      ),
  })
}

export function useRenameGitConnection() {
  const invalidate = useInvalidateGitConnections()
  return useMutation<
    GitProviderConnection,
    ApiError,
    { id: string; label: string }
  >({
    mutationFn: ({ id, label }) =>
      request(
        `/api/v1/git-connections/${encodeURIComponent(id)}`,
        json('PATCH', { label }),
        'rename failed',
      ),
    onSuccess: invalidate,
  })
}

export function useSetDefaultGitConnection() {
  const invalidate = useInvalidateGitConnections()
  return useMutation<GitProviderConnection, ApiError, string>({
    mutationFn: (id) =>
      request(
        `/api/v1/git-connections/${encodeURIComponent(id)}/default`,
        json('POST'),
        'set default failed',
      ),
    onSuccess: invalidate,
  })
}

export function useDisconnectGitConnection() {
  const invalidate = useInvalidateGitConnections()
  const queryClient = useQueryClient()
  return useMutation<GitDisconnectResult, ApiError, string>({
    mutationFn: (id) =>
      request(
        `/api/v1/git-connections/${encodeURIComponent(id)}`,
        json('DELETE'),
        'disconnect failed',
      ),
    onSuccess: () => {
      invalidate()
      void queryClient.invalidateQueries({ queryKey: gitSourceKeys.all })
    },
  })
}

export function useGitConnectionBranches(
  connectionId: string,
  repo: string,
  enabled: boolean,
) {
  return useQuery({
    queryKey: gitConnectionKeys.branches(connectionId, repo),
    queryFn: () =>
      request<GitConnectionBranch[]>(
        `/api/v1/git-connections/${encodeURIComponent(connectionId)}/branches?repo=${encodeURIComponent(repo)}`,
        undefined,
        'fetch branches failed',
      ),
    enabled: enabled && connectionId !== '' && repo !== '',
  })
}

// Connects the repository as appName's git source through the chosen
// connection and registers the push webhook with that connection's token.
export function connectRepoAsSource(
  connectionId: string,
  req: UseRepoAsSourceRequest,
): Promise<GitSourceResource> {
  return request(
    `/api/v1/git-connections/${encodeURIComponent(connectionId)}/use-as-source`,
    json('POST', req),
    'use repository as source failed',
  )
}
