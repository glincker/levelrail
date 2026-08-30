// Fetchers and hooks for internal/api/oauth.go and oauth_settings.go:
// public provider list, and the AbilityRoot-gated settings CRUD.

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface PublicOAuthProvider {
  provider: string
  enabled: boolean
  display_name?: string
}

export interface OAuthProviderSettings {
  provider: string
  enabled: boolean
  client_id?: string
  allowed_email_domain?: string
  issuer_url?: string
  display_name?: string
  has_client_secret: boolean
}

export interface UpdateOAuthProviderSettingsRequest {
  enabled: boolean
  client_id: string
  client_secret?: string
  allowed_email_domain: string
  issuer_url: string
  display_name: string
}

export const oauthKeys = {
  all: ['oauth'] as const,
  publicProviders: () => [...oauthKeys.all, 'public-providers'] as const,
  settings: () => [...oauthKeys.all, 'settings'] as const,
}

// GET /api/v1/auth/oauth/providers: public, unauthenticated, used by the
// login screen to decide which sign-in buttons to show.
export async function fetchPublicOAuthProviders(): Promise<
  PublicOAuthProvider[]
> {
  const res = await fetch('/api/v1/auth/oauth/providers')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch oauth providers failed: ${res.status}`),
    )
  }
  return (await res.json()) as PublicOAuthProvider[]
}

export function publicOAuthProvidersQueryOptions() {
  return queryOptions({
    queryKey: oauthKeys.publicProviders(),
    queryFn: fetchPublicOAuthProviders,
  })
}

// GET /api/v1/settings/oauth: the operator-facing settings page's read
// model, AbilityRoot-gated server-side.
export async function fetchOAuthSettings(): Promise<OAuthProviderSettings[]> {
  const res = await fetch('/api/v1/settings/oauth')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch oauth settings failed: ${res.status}`),
    )
  }
  return (await res.json()) as OAuthProviderSettings[]
}

export function oauthSettingsQueryOptions() {
  return queryOptions({
    queryKey: oauthKeys.settings(),
    queryFn: fetchOAuthSettings,
  })
}

export function useOAuthSettings() {
  return useSuspenseQuery(oauthSettingsQueryOptions())
}

export async function updateOAuthProviderSettings(
  provider: string,
  req: UpdateOAuthProviderSettingsRequest,
): Promise<OAuthProviderSettings> {
  const res = await fetch(
    `/api/v1/settings/oauth/${encodeURIComponent(provider)}`,
    {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    },
  )
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `update oauth settings failed: ${res.status}`),
    )
  }
  return (await res.json()) as OAuthProviderSettings
}

export function useUpdateOAuthProviderSettings() {
  const queryClient = useQueryClient()
  return useMutation<
    OAuthProviderSettings,
    ApiError,
    { provider: string; req: UpdateOAuthProviderSettingsRequest }
  >({
    mutationFn: ({ provider, req }) =>
      updateOAuthProviderSettings(provider, req),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: oauthKeys.settings() })
    },
  })
}
