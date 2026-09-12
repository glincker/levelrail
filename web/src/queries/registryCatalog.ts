// Query-key factory and fetchers for GET /api/v1/registry/repositories
// and GET /api/v1/registry/tags (internal/api/registry_catalog.go): the
// built-in registry's own repository/tag catalog, for the "existing
// image" app-creation step's repository/tag picker
// (RegistryImagePicker.tsx). Kept in its own module, the same "distinct
// resource, own file" reasoning queries/gitBranches.ts's own doc comment
// gives for a similarly narrow, form-scoped lookup.
//
// Both hooks degrade to "nothing to suggest" on failure or when the
// built-in registry isn't usable (disabled, no master key, no
// credentials yet: the API returns 501/409 for those), never blocking
// the underlying free-text image input: RegistryImagePicker itself
// simply doesn't render in that case, the same graceful-degradation
// shape useImageTagsOptional/useNodeListOptional already establish.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const registryCatalogKeys = {
  repositories: ['registry-catalog', 'repositories'] as const,
  tags: (repository: string) => ['registry-catalog', 'tags', repository] as const,
}

export async function fetchRegistryRepositories(): Promise<string[]> {
  const res = await fetch('/api/v1/registry/repositories')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `list registry repositories failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as { repositories: string[] | null } | null
  return body?.repositories ?? []
}

export function registryRepositoriesQueryOptions() {
  return queryOptions({
    queryKey: registryCatalogKeys.repositories,
    queryFn: fetchRegistryRepositories,
  })
}

// enabled is the caller's own signal for whether the built-in registry
// looks usable (from useRegistryStatus: enabled && status === 'running'):
// this hook never fires the request on its own until told to, the same
// "don't make a network call against a resource that isn't there yet"
// reasoning useGitBranches' own doc comment gives for its enabled gate.
export function useRegistryRepositoriesOptional(enabled: boolean) {
  return useQuery({ ...registryRepositoriesQueryOptions(), enabled, retry: false })
}

export async function fetchRegistryTags(repository: string): Promise<string[]> {
  const res = await fetch(`/api/v1/registry/tags?repository=${encodeURIComponent(repository)}`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `list registry tags failed: ${res.status}`),
    )
  }
  const body = (await res.json()) as { tags: string[] | null } | null
  return body?.tags ?? []
}

export function registryTagsQueryOptions(repository: string) {
  return queryOptions({
    queryKey: registryCatalogKeys.tags(repository),
    queryFn: () => fetchRegistryTags(repository),
  })
}

// repository is null until a repository has actually been picked, the
// same "nothing typed/picked yet, don't call" gate useGitBranches uses
// for repoUrl.
export function useRegistryTagsOptional(repository: string | null) {
  return useQuery({
    ...registryTagsQueryOptions(repository ?? ''),
    enabled: repository !== null && repository !== '',
    retry: false,
  })
}
