// /api/v1/settings/email/providers: the primary and backup senders
// (internal/api/email_providers.go). Credentials are write-only on the wire
// (only the *_set flags come back), and revealed values never enter this cache.

import {
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { emailSettingsKeys, type EmailBackend } from './emailSettings'

export const emailProvidersKey = ['email-settings', 'providers'] as const

export type EmailProviderRole = 'primary' | 'backup'

export interface EmailProvider {
  id?: string
  role?: EmailProviderRole
  name?: string
  enabled?: boolean
  backend: Exclude<EmailBackend, ''>
  tls_mode?: '' | 'starttls' | 'tls' | 'none'
  tls_skip_verify?: boolean
  smtp_host?: string
  smtp_port?: number
  smtp_username?: string
  smtp_from?: string
  smtp_password?: string
  smtp_password_set?: boolean
  ses_region?: string
  ses_access_key_id?: string
  ses_from?: string
  ses_secret_access_key?: string
  ses_secret_access_key_set?: boolean
  ses_smtp_port?: number
  ses_smtp_username?: string
  ses_smtp_password?: string
  ses_smtp_password_set?: boolean
  resend_from?: string
  resend_api_key?: string
  resend_api_key_set?: boolean
  postmark_server_token?: string
  postmark_server_token_set?: boolean
  sendgrid_api_key?: string
  sendgrid_api_key_set?: boolean
  mailgun_api_key?: string
  mailgun_api_key_set?: boolean
  mailgun_domain?: string
  mailgun_region?: string
  last_ok_at?: string
  last_error_kind?: string
}

const BASE = '/api/v1/settings/email/providers'

async function request<T>(
  method: 'GET' | 'POST' | 'PUT' | 'DELETE',
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `email provider request failed: ${res.status}`,
      ),
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export function emailProvidersQueryOptions() {
  return queryOptions({
    queryKey: emailProvidersKey,
    queryFn: async (): Promise<EmailProvider[]> => {
      const body = await request<{ providers?: EmailProvider[] }>('GET', BASE)
      return body.providers ?? []
    },
    staleTime: 15_000,
  })
}

export function useEmailProviders() {
  return useQuery(emailProvidersQueryOptions())
}

export function providerByRole(
  providers: EmailProvider[] | undefined,
  role: EmailProviderRole,
): EmailProvider | undefined {
  return providers?.find((p) => p.role === role)
}

// A provider write changes the mirrored settings row and the status header too.
function useRefreshAfterWrite() {
  const queryClient = useQueryClient()
  return () => {
    void queryClient.invalidateQueries({ queryKey: emailProvidersKey })
    void queryClient.invalidateQueries({ queryKey: emailSettingsKeys.all })
    void queryClient.invalidateQueries({ queryKey: emailSettingsKeys.status })
  }
}

export function useCreateEmailProvider() {
  const refresh = useRefreshAfterWrite()
  return useMutation({
    mutationFn: (p: EmailProvider) => request<EmailProvider>('POST', BASE, p),
    onSuccess: refresh,
  })
}

export function useUpdateEmailProvider() {
  const refresh = useRefreshAfterWrite()
  return useMutation({
    mutationFn: (p: EmailProvider & { id: string }) =>
      request<EmailProvider>('PUT', `${BASE}/${encodeURIComponent(p.id)}`, p),
    onSuccess: refresh,
  })
}

export function useDeleteEmailProvider() {
  const refresh = useRefreshAfterWrite()
  return useMutation({
    mutationFn: (id: string) =>
      request<undefined>('DELETE', `${BASE}/${encodeURIComponent(id)}`),
    onSuccess: refresh,
  })
}

export function usePromoteEmailProvider() {
  const refresh = useRefreshAfterWrite()
  return useMutation({
    mutationFn: (id: string) =>
      request<{ providers: EmailProvider[] }>(
        'POST',
        `${BASE}/${encodeURIComponent(id)}/promote`,
        {},
      ),
    onSuccess: refresh,
  })
}
