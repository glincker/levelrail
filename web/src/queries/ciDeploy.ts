// /api/v1/apps/{name}/ci/* (internal/api/ci_oidc.go): the generated deploy
// workflow and the repositories trusted to deploy through OIDC.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { appKeys } from './apps'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface CITrust {
  id: string
  provider: string
  issuer?: string
  repository: string
  ref_pattern: string
  environment: string
  protected_ref_only?: boolean
  created_at: string
  // Set by the API on create when the trust is less strict than the default.
  warning?: string
}

export interface CITrustInput {
  provider: string
  issuer?: string
  org_id?: string
  repository: string
  ref_pattern: string
  environment: string
  protected_ref_only?: boolean
}

export interface CIWorkflow {
  yaml: string
  api_url: string
  has_trust: boolean
  needs_dashboard_url: boolean
}

export const ciDeployKeys = {
  all: (appName: string) => [...appKeys.detail(appName), 'ci'] as const,
  trusts: (appName: string) =>
    [...ciDeployKeys.all(appName), 'trusts'] as const,
  workflow: (appName: string) =>
    [...ciDeployKeys.all(appName), 'workflow'] as const,
}

function base(appName: string) {
  return `/api/v1/apps/${encodeURIComponent(appName)}/ci`
}

async function request<T>(
  input: string,
  init: RequestInit | undefined,
  what: string,
): Promise<T> {
  const res = await fetch(input, init)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `${what} failed: ${res.status}`),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export function useCITrusts(appName: string) {
  return useSuspenseQuery(
    queryOptions({
      queryKey: ciDeployKeys.trusts(appName),
      queryFn: () =>
        request<CITrust[]>(`${base(appName)}/trusts`, undefined, 'list trusts'),
    }),
  )
}

export function useCIWorkflow(appName: string) {
  return useSuspenseQuery(
    queryOptions({
      queryKey: ciDeployKeys.workflow(appName),
      queryFn: () =>
        request<CIWorkflow>(
          `${base(appName)}/workflow`,
          undefined,
          'fetch workflow',
        ),
    }),
  )
}

export function useAddCITrust(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<CITrust, ApiError, CITrustInput>({
    mutationFn: (input) =>
      request<CITrust>(
        `${base(appName)}/trusts`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(input),
        },
        'add trust',
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ciDeployKeys.all(appName) }),
  })
}

export function useRemoveCITrust(appName: string) {
  const queryClient = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: (id) =>
      request<void>(
        `${base(appName)}/trusts/${encodeURIComponent(id)}`,
        { method: 'DELETE' },
        'remove trust',
      ),
    onSuccess: () =>
      queryClient.invalidateQueries({ queryKey: ciDeployKeys.all(appName) }),
  })
}
