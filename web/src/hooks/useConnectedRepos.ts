import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { gitProvidersQueryOptions } from '../queries/gitProviders'
import { useGitHubAppRepos } from '../queries/githubApp'
import { useGitLabAppProjects } from '../queries/gitlabApp'
import { useBitbucketAppRepos } from '../queries/bitbucketApp'
import { useGiteaAppRepos } from '../queries/giteaApp'
import {
  fromBitbucketRepo,
  fromGiteaRepo,
  fromGitHubRepo,
  fromGitLabProject,
  type NormalizedRepoOption,
} from '../lib/gitRepoOptions'
import type { GitProviderName } from '../types/gitProviders'

export interface ConnectedRepos {
  repos: NormalizedRepoOption[]
  connectedProviders: GitProviderName[]
  isLoading: boolean
  /** False until the provider list itself has loaded. */
  providersReady: boolean
  /** True when at least one connected provider failed to list. */
  hasError: boolean
}

/** Every repository the connected provider apps can see, as one list. */
export function useConnectedRepos(): ConnectedRepos {
  const providers = useQuery(gitProvidersQueryOptions())
  const connected = useMemo(
    () =>
      new Set(
        (providers.data ?? [])
          .filter((p) => p.connected)
          .map((p) => p.provider),
      ),
    [providers.data],
  )
  const github = useGitHubAppRepos(connected.has('github'))
  const gitlab = useGitLabAppProjects(connected.has('gitlab'))
  const bitbucket = useBitbucketAppRepos(connected.has('bitbucket'))
  const gitea = useGiteaAppRepos(connected.has('gitea'))

  const repos = useMemo(() => {
    const out: NormalizedRepoOption[] = []
    for (const r of github.data?.repos ?? []) out.push(fromGitHubRepo(r))
    for (const p of gitlab.data ?? []) out.push(fromGitLabProject(p))
    for (const r of bitbucket.data ?? []) out.push(fromBitbucketRepo(r))
    for (const r of gitea.data ?? []) out.push(fromGiteaRepo(r))
    return out
  }, [github.data, gitlab.data, bitbucket.data, gitea.data])

  const queries = [github, gitlab, bitbucket, gitea]
  return {
    repos,
    connectedProviders: [...connected],
    providersReady: !providers.isLoading,
    isLoading: providers.isLoading || queries.some((q) => q.isLoading),
    hasError:
      queries.some((q) => q.isError) || (github.data?.errors?.length ?? 0) > 0,
  }
}
