// Query-key factory, fetcher, and deploy-spec mutation for the
// multi-service app group resource: GET /api/v1/apps/{name}/group
// (internal/api/apps_group.go's handleGetAppGroup) and POST
// /api/v1/apps/{name}/deploy-spec (internal/api/apps_multi.go's
// handleDeploySpec).

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { deployKeys } from './deploys'
import { ApiError, readErrorMessage } from '../lib/apiError'
import type {
  AppGroup,
  DeploySpecInput,
  DeploySpecResult,
} from '../types/appGroup'

export const appGroupKeys = {
  detail: (appName: string) => [...appKeys.detail(appName), 'group'] as const,
}

export async function fetchAppGroup(appName: string): Promise<AppGroup> {
  const res = await fetch(`/api/v1/apps/${encodeURIComponent(appName)}/group`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch app group failed: ${res.status}`),
    )
  }
  return (await res.json()) as AppGroup
}

export function appGroupQueryOptions(appName: string) {
  return queryOptions({
    queryKey: appGroupKeys.detail(appName),
    queryFn: () => fetchAppGroup(appName),
  })
}

export function useAppGroup(appName: string) {
  return useSuspenseQuery(appGroupQueryOptions(appName))
}

function toDeploySpecServiceBody(svc: DeploySpecInput['services'][number]) {
  return {
    build: {
      type: svc.buildType,
      ...(svc.buildPath ? { path: svc.buildPath } : {}),
      ...(svc.buildImage ? { image: svc.buildImage } : {}),
    },
    ...(svc.port ? { port: svc.port } : {}),
    ...(svc.domains && svc.domains.length > 0 ? { domains: svc.domains } : {}),
    ...(svc.replicas ? { replicas: svc.replicas } : {}),
  }
}

export async function deploySpec(
  appName: string,
  input: DeploySpecInput,
): Promise<DeploySpecResult> {
  const services: Record<
    string,
    ReturnType<typeof toDeploySpecServiceBody>
  > = {}
  for (const svc of input.services) {
    services[svc.key.trim()] = toDeploySpecServiceBody(svc)
  }

  const res = await fetch(
    `/api/v1/apps/${encodeURIComponent(appName)}/deploy-spec`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        repo_url: input.repoUrl,
        ref: input.ref,
        ...(input.imageRepoBase
          ? { image_repo_base: input.imageRepoBase }
          : {}),
        services,
      }),
    },
  )
  // A partial per-service failure still comes back as a 2xx (207 Multi
  // Status), see deploySpecResponse's own doc comment
  // (internal/api/apps_multi.go): only a non-2xx status here means the
  // fan-out request itself was rejected, not that every service deployed.
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `deploy spec failed: ${res.status}`),
    )
  }
  return (await res.json()) as DeploySpecResult
}

export function useDeploySpec(appName: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: DeploySpecInput) => deploySpec(appName, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: appGroupKeys.detail(appName),
      })
      void queryClient.invalidateQueries({
        queryKey: deployKeys.status(appName),
      })
    },
  })
}
