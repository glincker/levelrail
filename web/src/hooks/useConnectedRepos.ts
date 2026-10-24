import { useEffect, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { gitProvidersQueryOptions } from '../queries/gitProviders'
import { useGitRepoList } from '../queries/gitRepos'
import {
  fromGitRepoItem,
  type NormalizedRepoOption,
} from '../lib/gitRepoOptions'
import type { GitProviderName, GitProviderStatus } from '../types/gitProviders'
import type {
  GitRepoConnectionFacet,
  GitRepoItem,
  GitRepoProvider,
} from '../types/gitRepos'

// A merged list is paged on the server; the new-app list pulls pages until
// this many pages are loaded, then leaves the rest to search.
const MAX_PAGES = 10

export interface ConnectedRepos {
  repos: NormalizedRepoOption[]
  connectedProviders: GitProviderName[]
  /** Capability and health of every provider, connected or not. */
  providers: GitProviderStatus[]
  /** Every account the merged list was built from. */
  connections: GitRepoConnectionFacet[]
  isLoading: boolean
  /** False until the provider list itself has loaded. */
  providersReady: boolean
  /** True when a connected account failed to list. */
  hasError: boolean
}

/** Every repository every connected account can see, from the per-connection aggregated listing. */
export function useConnectedRepos(): ConnectedRepos {
  const providers = useQuery(gitProvidersQueryOptions())
  const connected = useMemo(
    () =>
      (providers.data ?? []).filter((p) => p.connected).map((p) => p.provider),
    [providers.data],
  )
  const list = useGitRepoList('', {}, connected.length > 0)
  const { fetchNextPage, hasNextPage, isFetchingNextPage } = list
  const pageCount = list.data?.pages.length ?? 0
  useEffect(() => {
    if (hasNextPage && !isFetchingNextPage && pageCount < MAX_PAGES) {
      void fetchNextPage()
    }
  }, [hasNextPage, isFetchingNextPage, pageCount, fetchNextPage])

  const repos = useMemo(() => {
    const out: NormalizedRepoOption[] = []
    for (const page of list.data?.pages ?? []) {
      for (const item of page.items) {
        out.push(fromGitRepoItem(providerOf(item), item))
      }
    }
    return out
  }, [list.data])

  const first = list.data?.pages[0]
  return {
    repos,
    connectedProviders: connected,
    providers: providers.data ?? [],
    connections: first?.connections ?? [],
    providersReady: !providers.isLoading,
    isLoading:
      providers.isLoading ||
      list.isLoading ||
      (hasNextPage && pageCount < MAX_PAGES),
    hasError:
      list.isError ||
      (first?.errors?.length ?? 0) > 0 ||
      (first?.connections ?? []).some((c) => c.failed),
  }
}

function providerOf(item: GitRepoItem): GitRepoProvider {
  return item.provider ?? 'github'
}
