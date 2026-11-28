// Query-key factory and fetchers for the /registry-credentials resource
// (internal/api/registry_credentials.go), following the same shared-
// queryOptions pattern queries/backupTargets.ts already established.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type {
  CreateRegistryCredentialRequest,
  RegistryCredential,
  RegistryPresetRequest,
  RegistryPresetTestResult,
} from '../types/registryCredential'
import type {
  DockerHubLimits,
  DockerHubStatus,
} from '../types/dockerHubAccount'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const registryCredentialKeys = {
  all: ['registry-credentials'] as const,
  list: () => [...registryCredentialKeys.all, 'list'] as const,
  repositories: (id: string) =>
    [...registryCredentialKeys.all, 'repositories', id] as const,
  tags: (id: string, repository: string) =>
    [...registryCredentialKeys.all, 'tags', id, repository] as const,
  dockerHub: (id: string) =>
    [...registryCredentialKeys.all, 'dockerhub', id] as const,
}

// GET /api/v1/registry-credentials (handleListRegistryCredentials): no
// 501 case, just lists whatever rows already exist.
export async function fetchRegistryCredentials(): Promise<
  RegistryCredential[]
> {
  const res = await fetch('/api/v1/registry-credentials')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch registry credentials failed: ${res.status}`,
      ),
    )
  }
  return ((await res.json()) as RegistryCredential[] | null) ?? []
}

export function registryCredentialListQueryOptions() {
  return queryOptions({
    queryKey: registryCredentialKeys.list(),
    queryFn: fetchRegistryCredentials,
  })
}

export function useRegistryCredentials() {
  return useSuspenseQuery(registryCredentialListQueryOptions())
}

// Non-suspending variant for callers that want the credential list as a
// supplementary signal without making an unrelated tree's whole Suspense
// boundary wait on it, the same reasoning queries/registry.ts's
// useRegistryStatus already uses.
export function useRegistryCredentialsOptional() {
  return useQuery(registryCredentialListQueryOptions())
}

// POST /api/v1/registry-credentials (handleCreateRegistryCredential).
// 501 means the control plane was started without APP_MASTER_KEY, the
// same server-configuration-gap case backupTargets.ts's createBackupTarget
// carries for the identical reason (both route through internal/secrets).
export async function createRegistryCredential(
  req: CreateRegistryCredentialRequest,
): Promise<RegistryCredential> {
  const res = await fetch('/api/v1/registry-credentials', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (res.status === 501) {
    throw new ApiError(
      501,
      'Registry credentials require a master key to be configured on this control plane.',
    )
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `create registry credential failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as RegistryCredential
}

export function useCreateRegistryCredential() {
  const queryClient = useQueryClient()
  return useMutation<
    RegistryCredential,
    ApiError,
    CreateRegistryCredentialRequest
  >({
    mutationFn: createRegistryCredential,
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: registryCredentialKeys.list(),
      })
    },
  })
}

// DELETE /api/v1/registry-credentials/{id} (handleDeleteRegistryCredential).
// 204 on success, no body to parse.
export async function deleteRegistryCredential(id: string): Promise<void> {
  const res = await fetch(
    `/api/v1/registry-credentials/${encodeURIComponent(id)}`,
    { method: 'DELETE' },
  )
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(
      res,
      `delete registry credential failed: ${res.status}`,
    ),
  )
}

export function useDeleteRegistryCredential() {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: deleteRegistryCredential,
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: registryCredentialKeys.list(),
      })
    },
  })
}

// POST /api/v1/registry-credentials/{id}/test
// (handleTestRegistryCredential): authenticates the stored credential
// against its registry host, without pulling anything. 204 on success,
// 502 with a specific reason on failure, the same shape
// notificationChannels.ts's testExistingNotificationChannel already
// establishes for its own on-demand verification.
export async function testRegistryCredential(id: string): Promise<void> {
  const res = await fetch(
    `/api/v1/registry-credentials/${encodeURIComponent(id)}/test`,
    { method: 'POST' },
  )
  if (res.status === 204) {
    return
  }
  throw new ApiError(
    res.status,
    await readErrorMessage(
      res,
      `test registry credential failed: ${res.status}`,
    ),
  )
}

export function useTestRegistryCredential() {
  return useMutation<void, ApiError, string>({
    mutationFn: testRegistryCredential,
  })
}

// GET /api/v1/registry-credentials/{id}/repositories
// (handleListRegistryCredentialRepositories): every repository in the
// external registry this credential authenticates against. Same
// graceful-degradation shape queries/registryCatalog.ts establishes for
// the built-in registry's own catalog: a failed lookup here should never
// block the browse dialog from rendering, just leave the list empty with
// the error shown alongside it.
export async function fetchRegistryCredentialRepositories(
  id: string,
): Promise<string[]> {
  const res = await fetch(
    `/api/v1/registry-credentials/${encodeURIComponent(id)}/repositories`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `list registry credential repositories failed: ${res.status}`,
      ),
    )
  }
  const body = (await res.json()) as { repositories: string[] | null } | null
  return body?.repositories ?? []
}

export function registryCredentialRepositoriesQueryOptions(id: string) {
  return queryOptions({
    queryKey: registryCredentialKeys.repositories(id),
    queryFn: () => fetchRegistryCredentialRepositories(id),
  })
}

// enabled lets the caller defer the request until a browse dialog is
// actually open, the same "don't call until told to" gate
// useRegistryRepositoriesOptional already establishes.
export function useRegistryCredentialRepositories(
  id: string,
  enabled: boolean,
) {
  return useQuery({
    ...registryCredentialRepositoriesQueryOptions(id),
    enabled,
    retry: false,
  })
}

// GET /api/v1/registry-credentials/{id}/tags?repository=<name>
// (handleListRegistryCredentialTags): every tag pushed for one
// repository in the external registry this credential authenticates
// against.
export async function fetchRegistryCredentialTags(
  id: string,
  repository: string,
): Promise<string[]> {
  const res = await fetch(
    `/api/v1/registry-credentials/${encodeURIComponent(id)}/tags?repository=${encodeURIComponent(repository)}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `list registry credential tags failed: ${res.status}`,
      ),
    )
  }
  const body = (await res.json()) as { tags: string[] | null } | null
  return body?.tags ?? []
}

export function registryCredentialTagsQueryOptions(
  id: string,
  repository: string,
) {
  return queryOptions({
    queryKey: registryCredentialKeys.tags(id, repository),
    queryFn: () => fetchRegistryCredentialTags(id, repository),
  })
}

// repository is null until a repository has actually been picked in the
// browse dialog, the same "nothing picked yet, don't call" gate
// useRegistryTagsOptional already establishes for the built-in registry.
export function useRegistryCredentialTags(
  id: string,
  repository: string | null,
  enabled: boolean,
) {
  return useQuery({
    ...registryCredentialTagsQueryOptions(id, repository ?? ''),
    enabled: enabled && repository !== null && repository !== '',
    retry: false,
  })
}

// POST /api/v1/registry-credentials/presets/test: mints a login to prove the
// cloud connection works; nothing is stored and no password comes back.
export async function testRegistryPreset(
  req: RegistryPresetRequest,
): Promise<RegistryPresetTestResult> {
  const res = await fetch('/api/v1/registry-credentials/presets/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `test registry preset failed: ${res.status}`),
    )
  }
  return (await res.json()) as RegistryPresetTestResult
}

export function useTestRegistryPreset() {
  return useMutation<RegistryPresetTestResult, ApiError, RegistryPresetRequest>(
    { mutationFn: testRegistryPreset },
  )
}

// POST /api/v1/registry-credentials/{id}/refresh: mint a new login now.
export async function refreshRegistryCredential(
  id: string,
): Promise<RegistryCredential> {
  const res = await fetch(
    `/api/v1/registry-credentials/${encodeURIComponent(id)}/refresh`,
    { method: 'POST' },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `refresh registry credential failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as RegistryCredential
}

export function useRefreshRegistryCredential() {
  const queryClient = useQueryClient()
  return useMutation<RegistryCredential, ApiError, string>({
    mutationFn: refreshRegistryCredential,
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: registryCredentialKeys.list(),
      })
    },
  })
}

// GET /api/v1/registry-credentials/{id}/dockerhub: token status plus pull
// limits; refresh=true bypasses the server-side cache and re-checks Docker Hub.
export async function fetchDockerHubStatus(
  id: string,
  refresh: boolean,
): Promise<DockerHubStatus> {
  const suffix = refresh ? '?refresh=true' : ''
  const res = await fetch(
    `/api/v1/registry-credentials/${encodeURIComponent(id)}/dockerhub${suffix}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `docker hub status failed: ${res.status}`),
    )
  }
  return (await res.json()) as DockerHubStatus
}

export function dockerHubStatusQueryOptions(id: string) {
  return queryOptions({
    queryKey: registryCredentialKeys.dockerHub(id),
    queryFn: () => fetchDockerHubStatus(id, false),
    retry: false,
  })
}

export function useDockerHubStatus(id: string) {
  return useQuery(dockerHubStatusQueryOptions(id))
}

// Re-test: forces a fresh status check and replaces the cached reading.
export function useRetestDockerHub(id: string) {
  const queryClient = useQueryClient()
  return useMutation<DockerHubStatus, ApiError, void>({
    mutationFn: () => fetchDockerHubStatus(id, true),
    onSuccess: (data) => {
      queryClient.setQueryData(registryCredentialKeys.dockerHub(id), data)
    },
  })
}

// GET /api/v1/registry-credentials/{id}/limits?refresh=true: only the pull
// limit, merged into the cached status so the meter updates in place.
export async function fetchDockerHubLimits(
  id: string,
  refresh: boolean,
): Promise<DockerHubLimits> {
  const suffix = refresh ? '?refresh=true' : ''
  const res = await fetch(
    `/api/v1/registry-credentials/${encodeURIComponent(id)}/limits${suffix}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `docker hub limits failed: ${res.status}`),
    )
  }
  return (await res.json()) as DockerHubLimits
}

export function useRefreshDockerHubLimits(id: string) {
  const queryClient = useQueryClient()
  return useMutation<DockerHubLimits, ApiError, void>({
    mutationFn: () => fetchDockerHubLimits(id, true),
    onSuccess: (limits) => {
      queryClient.setQueryData<DockerHubStatus>(
        registryCredentialKeys.dockerHub(id),
        (prev) => (prev ? { ...prev, limits } : prev),
      )
    },
  })
}

export interface ReplaceDockerHubTokenRequest {
  id: string
  name: string
  registry_host: string
  username: string
  password: string
}

// PUT /api/v1/registry-credentials/{id}: the server verifies a Docker Hub
// token before storing it and answers 400 when Docker Hub rejects it.
export async function replaceDockerHubToken(
  req: ReplaceDockerHubTokenRequest,
): Promise<RegistryCredential> {
  const { id, ...body } = req
  const res = await fetch(
    `/api/v1/registry-credentials/${encodeURIComponent(id)}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `replace token failed: ${res.status}`),
    )
  }
  return (await res.json()) as RegistryCredential
}

export function useReplaceDockerHubToken() {
  const queryClient = useQueryClient()
  return useMutation<
    RegistryCredential,
    ApiError,
    ReplaceDockerHubTokenRequest
  >({
    mutationFn: replaceDockerHubToken,
    onSuccess: (_data, vars) => {
      void queryClient.invalidateQueries({
        queryKey: registryCredentialKeys.dockerHub(vars.id),
      })
      void queryClient.invalidateQueries({
        queryKey: registryCredentialKeys.list(),
      })
    },
  })
}
