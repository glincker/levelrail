// Query-key factory and fetcher for GET /api/v1/pipelines/oidc: whether
// pipeline jobs can mint OIDC tokens on this control plane, and the
// URLs an operator wires into a cloud provider's OIDC trust policy.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface PipelineOIDCInfo {
  configured: boolean
  issuer_url?: string
  jwks_url?: string
  rotation_supported: boolean
}

export interface PipelineOIDCRotation {
  old_kid: string
  new_kid: string
  retire_at: string
  retiring_count: number
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

export async function rotatePipelineOIDCKey(
  retireAfter?: string,
): Promise<PipelineOIDCRotation> {
  const res = await fetch('/api/v1/pipelines/oidc/rotate-key', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(retireAfter ? { retire_after: retireAfter } : {}),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `rotate pipeline oidc key failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as PipelineOIDCRotation
}

export function useRotatePipelineOIDCKey() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: rotatePipelineOIDCKey,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: pipelineOidcKeys.all })
    },
  })
}
