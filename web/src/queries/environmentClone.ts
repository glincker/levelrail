// Query-key factory, fetchers, and mutation hook for
// GET /api/v1/environments/{id}/clone/preview and
// POST /api/v1/environments/{id}/clone (internal/api/environment_clone.go).
// Kept in its own module, the same "genuinely different resource"
// reasoning queries/promote.ts's own doc comment already gives for
// promotion vs. plain deploys.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import type {
  EnvironmentCloneAppInput,
  EnvironmentClonePreviewResource,
  EnvironmentCloneResultResource,
} from '../types/environmentClone'
import { environmentKeys } from './environments'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const environmentCloneKeys = {
  all: ['environmentClone'] as const,
  preview: (id: string, newName: string) =>
    [...environmentCloneKeys.all, 'preview', id, newName] as const,
}

export async function fetchEnvironmentClonePreview(
  id: string,
  newEnvironmentName: string,
): Promise<EnvironmentClonePreviewResource> {
  const params = new URLSearchParams({
    new_environment_name: newEnvironmentName,
  })
  const res = await fetch(
    `/api/v1/environments/${encodeURIComponent(id)}/clone/preview?${params.toString()}`,
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `preview environment clone failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as EnvironmentClonePreviewResource
}

// Not a suspense query: this feeds CloneEnvironmentDialog's own preview
// panel, which only exists once a new environment name has been typed
// inside an already-open dialog, the same "enabled once a prerequisite
// is chosen" shape usePromotePreview already has for its own dialog.
export function useEnvironmentClonePreview(id: string, newName: string) {
  return useQuery({
    queryKey: environmentCloneKeys.preview(id, newName),
    queryFn: () => fetchEnvironmentClonePreview(id, newName),
    enabled: id !== '' && newName.trim() !== '',
    retry: false,
  })
}

export interface CloneEnvironmentInput {
  newEnvironmentName: string
  copySecretValues: boolean
  apps: EnvironmentCloneAppInput[]
}

export async function cloneEnvironment(
  id: string,
  input: CloneEnvironmentInput,
): Promise<EnvironmentCloneResultResource> {
  const res = await fetch(
    `/api/v1/environments/${encodeURIComponent(id)}/clone`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        new_environment_name: input.newEnvironmentName,
        copy_secret_values: input.copySecretValues,
        apps: input.apps,
      }),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `clone environment failed: ${res.status}`),
    )
  }
  return (await res.json()) as EnvironmentCloneResultResource
}

export function useCloneEnvironment(projectId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, input }: { id: string; input: CloneEnvironmentInput }) =>
      cloneEnvironment(id, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: environmentKeys.list(projectId),
      })
      void queryClient.invalidateQueries({ queryKey: appKeys.list() })
    },
  })
}
