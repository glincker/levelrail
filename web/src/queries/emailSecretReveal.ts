// POST /api/v1/settings/email/providers/{id}/secrets/{field}/reveal
// (internal/api/email_secret_reveal.go). The value is returned to the caller
// only: it is never put in a query key or a cache entry.

import { useMutation, useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { EnvApiError } from './envProtection'

export const EMAIL_SECRET_STEP_UP_APP = 'email-secret'

export type EmailSecretField =
  | 'smtp_password'
  | 'ses_secret_access_key'
  | 'ses_smtp_password'
  | 'resend_api_key'
  | 'postmark_server_token'
  | 'sendgrid_api_key'
  | 'mailgun_api_key'

export function emailSecretStepUpScope(providerId: string) {
  return {
    action: 'manage',
    app: `${EMAIL_SECRET_STEP_UP_APP}:${providerId}`,
  } as const
}

// Throws EnvApiError so useStepUpGate can catch step_up_required.
export async function revealEmailSecret(
  providerId: string,
  field: EmailSecretField,
): Promise<string> {
  const res = await fetch(
    `/api/v1/settings/email/providers/${encodeURIComponent(providerId)}/secrets/${encodeURIComponent(field)}/reveal`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: '{}',
      cache: 'no-store',
    },
  )
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as {
      error?: string
      code?: string
    } | null
    throw new EnvApiError(
      res.status,
      body?.error ?? `reveal failed: ${res.status}`,
      body?.code ?? '',
    )
  }
  const data = (await res.json()) as { value: string }
  return data.value
}

// gcTime 0 and the caller resets the mutation, so the value outlives the
// request only in the component's own state.
export function useRevealEmailSecret(providerId: string) {
  return useMutation({
    gcTime: 0,
    mutationFn: (field: EmailSecretField) =>
      revealEmailSecret(providerId, field),
  })
}

interface ProviderRow {
  id?: string
  role?: string
}

async function fetchPrimaryProviderId(): Promise<string | null> {
  const res = await fetch('/api/v1/settings/email/providers')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `fetch email providers failed: ${res.status}`,
      ),
    )
  }
  const body = (await res.json()) as { providers?: ProviderRow[] }
  return body.providers?.find((p) => p.role === 'primary')?.id ?? null
}

// The settings card edits the primary provider, so its reveal buttons target it.
export function usePrimaryEmailProviderId() {
  return useQuery({
    queryKey: ['email-settings', 'primary-provider-id'] as const,
    queryFn: fetchPrimaryProviderId,
    staleTime: 60_000,
  })
}
