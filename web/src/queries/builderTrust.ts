// Builder trust: PUT /api/v1/nodes/{id}/build-trust and GET/PUT
// /api/v1/environments/{id}/builders (internal/api/builder_trust.go).

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { buildRunnerKeys } from './buildRunners'

export const BUILD_TRUST_TIERS = ['server', 'managed', 'personal'] as const
export type BuildTrustTier = (typeof BUILD_TRUST_TIERS)[number]

export function isBuildTrustTier(value: string): value is BuildTrustTier {
  return (BUILD_TRUST_TIERS as readonly string[]).includes(value)
}

export interface EnvironmentBuilders {
  environment_id: string
  kind: string
  // Tier names and node ids, as stored.
  allowed_builders: string[]
  personal_allowed: boolean
}

export const environmentBuildersKeys = {
  one: (envId: string) => ['environment-builders', envId] as const,
}

async function json<T>(res: Response, what: string): Promise<T> {
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

function buildersUrl(envId: string): string {
  return `/api/v1/environments/${encodeURIComponent(envId)}/builders`
}

export function environmentBuildersOptions(envId: string) {
  return queryOptions({
    queryKey: environmentBuildersKeys.one(envId),
    queryFn: async ({ signal }) =>
      json<EnvironmentBuilders>(
        await fetch(buildersUrl(envId), { signal }),
        'fetch environment builders',
      ),
    retry: false,
  })
}

export function useEnvironmentBuilders(envId: string) {
  return useQuery(environmentBuildersOptions(envId))
}

export function useSaveEnvironmentBuilders(envId: string) {
  const queryClient = useQueryClient()
  return useMutation<EnvironmentBuilders, ApiError, string[]>({
    mutationFn: async (allowed) =>
      json<EnvironmentBuilders>(
        await fetch(buildersUrl(envId), {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ allowed_builders: allowed }),
        }),
        'save environment builders',
      ),
    onSuccess: (view) => {
      queryClient.setQueryData(environmentBuildersKeys.one(envId), view)
    },
  })
}

export function useSetNodeBuildTrust() {
  const queryClient = useQueryClient()
  return useMutation<
    { id: string; build_trust: string },
    ApiError,
    { id: string; tier: BuildTrustTier }
  >({
    mutationFn: async ({ id, tier }) =>
      json(
        await fetch(`/api/v1/nodes/${encodeURIComponent(id)}/build-trust`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ build_trust: tier }),
        }),
        'set build trust',
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: buildRunnerKeys.all })
    },
  })
}
