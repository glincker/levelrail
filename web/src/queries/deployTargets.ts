import { queryOptions, useQuery } from '@tanstack/react-query'
import type { DeployTargetGroup } from '../types/deployTargets'
import { ApiError } from '../lib/apiError'

export const deployTargetKeys = {
  all: ['deploy-targets'] as const,
}

export async function fetchDeployTargets(): Promise<DeployTargetGroup[]> {
  const res = await fetch('/api/v1/deploy-targets')
  if (!res.ok) {
    throw new ApiError(res.status, `deploy targets failed: ${res.status}`)
  }
  return (await res.json()) as DeployTargetGroup[]
}

// Optional like useNodeListOptional: a failure means callers fall back to the plain node list.
export function useDeployTargetsOptional() {
  return useQuery({
    ...queryOptions({
      queryKey: deployTargetKeys.all,
      queryFn: fetchDeployTargets,
    }),
    retry: false,
  })
}
