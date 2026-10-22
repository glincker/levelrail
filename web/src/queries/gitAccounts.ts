import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { buildGitAccounts, type GitAccount } from '../lib/gitAccounts'
import { gitConnectionsQueryOptions } from './gitConnections'
import { fetchGitRepoList, gitRepoKeys } from './gitRepos'
import { gitProvidersQueryOptions } from './gitProviders'
import { githubAppKeys, fetchGitHubAppInstallations } from './githubApp'

const FACET_PROBE_PARAMS = { limit: 1 } as const

export interface GitAccountsState {
  accounts: GitAccount[]
  isLoading: boolean
  isError: boolean
  lastSyncAt?: string
  githubAddUrl?: string
}

// One merged list of every connected account across hosts. The repo probe is
// best effort: it only supplies repo counts and the last sync time.
export function useGitAccounts(): GitAccountsState {
  const providers = useQuery(gitProvidersQueryOptions())
  const connections = useQuery(gitConnectionsQueryOptions())
  const githubConnected = (providers.data ?? []).some(
    (p) => p.provider === 'github' && p.connected,
  )
  const installations = useQuery({
    queryKey: githubAppKeys.installations(),
    queryFn: fetchGitHubAppInstallations,
    enabled: githubConnected,
  })
  const probe = useQuery({
    queryKey: [...gitRepoKeys.provider(''), FACET_PROBE_PARAMS],
    queryFn: () => fetchGitRepoList('', FACET_PROBE_PARAMS),
    enabled:
      providers.isSuccess && (providers.data ?? []).some((p) => p.connected),
    retry: false,
  })

  const accounts = useMemo(
    () =>
      buildGitAccounts({
        providers: providers.data ?? [],
        connections: connections.data ?? [],
        installations: installations.data?.installations ?? [],
        facets: probe.data?.connections ?? [],
      }),
    [providers.data, connections.data, installations.data, probe.data],
  )
  return {
    accounts,
    isLoading: providers.isLoading || connections.isLoading,
    isError: providers.isError && connections.isError,
    lastSyncAt: probe.data?.cached_at,
    githubAddUrl: installations.data?.add_org_url,
  }
}
