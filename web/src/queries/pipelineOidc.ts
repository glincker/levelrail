// Query-key factory and fetcher for GET /api/v1/pipelines/oidc: whether
// pipeline jobs can mint OIDC tokens on this control plane, and the
// URLs an operator wires into a cloud provider's OIDC trust policy.

import { queryOptions, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface PipelineOIDCInfo {
  configured: boolean
  issuer_url?: string
  jwks_url?: string
}

export const pipelineOidcKeys = {
  all: ['pipeline-oidc'] as const,
}

export async function fetchPipelineOIDCInfo(): Promise<PipelineOIDCInfo> {
  const res = await fetch('/api/v1/pipelines/oidc')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch pipeline oidc info failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as PipelineOIDCInfo
}

export function pipelineOidcQueryOptions() {
  return queryOptions({
    queryKey: pipelineOidcKeys.all,
    queryFn: fetchPipelineOIDCInfo,
  })
}

export function usePipelineOIDCInfo() {
  return useQuery(pipelineOidcQueryOptions())
}
