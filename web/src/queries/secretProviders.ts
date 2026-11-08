// /api/v1/settings/secret-providers (internal/api/routes_secretprovider.go).
// The token is write-only; has_token reports presence.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export type SecretProviderType = 'doppler' | 'infisical'

export interface SecretProvider {
  name: string
  type: SecretProviderType
  project: string
  environment: string
  path?: string
  base_url?: string
  self_hosted: boolean
  has_token: boolean
  updated_at: string
  rotation_grace_until?: string
}

export interface PutSecretProviderRequest {
  type: SecretProviderType
  project: string
  environment: string
  path?: string
  base_url?: string
  // Empty or omitted keeps the stored token.
  token?: string
}

export interface SecretProviderTestResult {
  ok: boolean
  reason?: string
}

export const secretProviderKeys = {
  all: ['secret-providers'] as const,
}

const BASE = '/api/v1/settings/secret-providers'

async function failure(res: Response, what: string): Promise<ApiError> {
  return new ApiError(
    res.status,
    await readErrorMessage(res, `${what} failed: ${res.status}`),
  )
}

export function secretProvidersQueryOptions() {
  return queryOptions({
    queryKey: secretProviderKeys.all,
    queryFn: async (): Promise<SecretProvider[]> => {
      const res = await fetch(BASE)
      if (!res.ok) throw await failure(res, 'list secret providers')
      const body = (await res.json()) as { providers: SecretProvider[] }
      return body.providers
    },
    staleTime: 30_000,
  })
}

export function useSecretProviders() {
  return useQuery(secretProvidersQueryOptions())
}

export function usePutSecretProvider() {
  const qc = useQueryClient()
  return useMutation<
    SecretProvider,
    ApiError,
    { name: string; body: PutSecretProviderRequest }
  >({
    mutationFn: async ({ name, body }) => {
      const res = await fetch(`${BASE}/${encodeURIComponent(name)}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      if (!res.ok) throw await failure(res, 'save secret provider')
      return (await res.json()) as SecretProvider
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: secretProviderKeys.all }),
  })
}

export function useDeleteSecretProvider() {
  const qc = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: async (name) => {
      const res = await fetch(`${BASE}/${encodeURIComponent(name)}`, {
        method: 'DELETE',
      })
      if (!res.ok) throw await failure(res, 'remove secret provider')
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: secretProviderKeys.all }),
  })
}

export function useTestSecretProvider() {
  return useMutation<SecretProviderTestResult, ApiError, string>({
    mutationFn: async (name) => {
      const res = await fetch(`${BASE}/${encodeURIComponent(name)}/test`, {
        method: 'POST',
      })
      if (!res.ok) throw await failure(res, 'test secret provider')
      return (await res.json()) as SecretProviderTestResult
    },
  })
}
