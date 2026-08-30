// Query-key factory and fetchers for the /environments resource
// (internal/api/environments.go), scoped to one project the same way
// queries/scheduledTasks.ts scopes to one app. Every key below is keyed
// by project id: there is no unscoped "list every environment" endpoint.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import type { EnvironmentResource } from '../types/environment'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const environmentKeys = {
  all: ['environments'] as const,
  list: (projectId: string) =>
    [...environmentKeys.all, 'list', projectId] as const,
  // No fetch-by-id endpoint exists (this file's own doc comment), so
  // "detail" is only ever a query-key namespace, e.g. for
  // queries/environmentEnv.ts's own shared env vars, never a fetcher of
  // its own.
  detail: (id: string) => [...environmentKeys.all, 'detail', id] as const,
}

export async function fetchEnvironments(
  projectId: string,
): Promise<EnvironmentResource[]> {
  const res = await fetch(
    `/api/v1/projects/${encodeURIComponent(projectId)}/environments`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch environments failed: ${res.status}`),
    )
  }
  return (await res.json()) as EnvironmentResource[]
}

export function environmentListQueryOptions(projectId: string) {
  return queryOptions({
    queryKey: environmentKeys.list(projectId),
    queryFn: () => fetchEnvironments(projectId),
    enabled: projectId !== '',
  })
}

export function useEnvironments(projectId: string) {
  return useSuspenseQuery(environmentListQueryOptions(projectId))
}

// For an optional picker inside another flow (MoveToEnvironmentDialog on
// an app's Overview): mirrors useProjectListOptional's own doc comment.
// A failure, or an app with no project at all, degrades to "no
// environments to pick from" rather than blocking the surrounding form.
export function useEnvironmentListOptional(projectId: string) {
  return useQuery({ ...environmentListQueryOptions(projectId), retry: false })
}

export async function createEnvironment(
  projectId: string,
  name: string,
): Promise<EnvironmentResource> {
  const res = await fetch(
    `/api/v1/projects/${encodeURIComponent(projectId)}/environments`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `create environment failed: ${res.status}`),
    )
  }
  return (await res.json()) as EnvironmentResource
}

export function useCreateEnvironment(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => createEnvironment(projectId, name),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: environmentKeys.list(projectId),
      })
    },
  })
}

// DELETE /api/v1/environments/{id} (handleDeleteEnvironment): any app
// tagged with it survives, simply untagged again (the backend's ON
// DELETE SET NULL foreign key), the same reasoning useDeleteProject's
// own doc comment gives for its resource.
export async function deleteEnvironment(id: string): Promise<void> {
  const res = await fetch(`/api/v1/environments/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `delete environment failed: ${res.status}`),
    )
  }
}

export function useDeleteEnvironment(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: deleteEnvironment,
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: environmentKeys.list(projectId),
      })
    },
  })
}
