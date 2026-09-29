// Query-key factory and fetchers for GET /api/v1/dockerhub/search and GET
// /api/v1/dockerhub/repositories/{namespace}/{repo}/tags
// (internal/api/dockerhub_search.go): the picker's third source
// (RegistryImagePicker.tsx), searching public Docker Hub for a
// well-known image that isn't in the built-in registry or a connected
// credential. Same shared-queryOptions module shape queries/
// registryCatalog.ts already establishes for the built-in registry's own
// catalog.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface DockerHubRepository {
  repo_name: string
  short_description: string
  star_count: number
  is_official: boolean
  is_automated: boolean
}

export interface DockerHubTag {
  name: string
}

export const dockerHubKeys = {
  search: (query: string) => ['dockerhub', 'search', query] as const,
  tags: (namespace: string, repository: string) =>
    ['dockerhub', 'tags', namespace, repository] as const,
}

// splitDockerHubRepoName turns Docker Hub search's own repo_name (e.g.
// "postgres" for an official image, "bitnami/postgresql" for a
// namespaced one) into the {namespace, repo} path segments the tags
// endpoint needs, defaulting an absent namespace to "library" the same
// way the backend's own ListTags does for the identical case.
export function splitDockerHubRepoName(repoName: string): {
  namespace: string
  repo: string
} {
  const slash = repoName.indexOf('/')
  if (slash === -1) return { namespace: 'library', repo: repoName }
  return {
    namespace: repoName.slice(0, slash),
    repo: repoName.slice(slash + 1),
  }
}

export async function fetchDockerHubSearch(
  query: string,
): Promise<DockerHubRepository[]> {
  const res = await fetch(
    `/api/v1/dockerhub/search?q=${encodeURIComponent(query)}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `docker hub search failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as {
    results: DockerHubRepository[] | null
  } | null
  return body?.results ?? []
}

export function dockerHubSearchQueryOptions(query: string) {
  return queryOptions({
    queryKey: dockerHubKeys.search(query),
    queryFn: () => fetchDockerHubSearch(query),
  })
}

// query is the caller's already-debounced search term: this hook fires
// on every change, so debouncing belongs upstream (useDebouncedValue in
// RegistryImagePicker), the same division of concerns
// useRegistryTagsOptional's "repository is null until picked" gate draws
// for its own enabled condition.
export function useDockerHubSearch(query: string) {
  return useQuery({
    ...dockerHubSearchQueryOptions(query),
    enabled: query.trim() !== '',
    retry: false,
  })
}

export async function fetchDockerHubTags(
  namespace: string,
  repository: string,
): Promise<DockerHubTag[]> {
  const res = await fetch(
    `/api/v1/dockerhub/repositories/${encodeURIComponent(namespace)}/${encodeURIComponent(repository)}/tags`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `docker hub list tags failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as { tags: DockerHubTag[] | null } | null
  return body?.tags ?? []
}

export function dockerHubTagsQueryOptions(
  namespace: string,
  repository: string,
) {
  return queryOptions({
    queryKey: dockerHubKeys.tags(namespace, repository),
    queryFn: () => fetchDockerHubTags(namespace, repository),
  })
}

// repository is null until a search result has actually been picked, the
// same "nothing picked yet, don't call" gate useRegistryTagsOptional
// already establishes.
export function useDockerHubTags(namespace: string, repository: string | null) {
  return useQuery({
    ...dockerHubTagsQueryOptions(namespace, repository ?? ''),
    enabled: repository !== null && repository !== '',
    retry: false,
  })
}
