// GET/PUT /api/v1/settings/email and the status, test, domain-check and
// delivery log endpoints beside it (internal/api/email_settings.go,
// internal/api/email_delivery.go).

import {
  infiniteQueryOptions,
  queryOptions,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const emailSettingsKeys = {
  all: ['email-settings'] as const,
  status: ['email-settings', 'status'] as const,
  domain: ['email-settings', 'domain-check'] as const,
  deliveries: (f: EmailDeliveryFilters) =>
    ['email-settings', 'deliveries', f] as const,
}

export type EmailBackend =
  | ''
  | 'smtp'
  | 'ses'
  | 'ses_smtp'
  | 'resend'
  | 'postmark'
  | 'sendgrid'
  | 'mailgun'

// EmailSettings mirrors emailSettingsResource exactly. Every credential
// is write-only: only ever sent, never present in a GET response (the
// *_set flags report presence instead).
export interface EmailSettings {
  backend: EmailBackend
  from_name?: string
  from_address?: string
  reply_to?: string
  envelope_from?: string
  sends_per_minute?: number
  sends_per_day?: number
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
  ses_regions?: string[]
}

export async function fetchEmailSettings(): Promise<EmailSettings> {
  const res = await fetch('/api/v1/settings/email')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `fetch email settings failed: ${res.status}`),
    )
  }
  return (await res.json()) as EmailSettings
}

export function emailSettingsQueryOptions() {
  return queryOptions({
    queryKey: emailSettingsKeys.all,
    queryFn: fetchEmailSettings,
    staleTime: 60_000,
  })
}

export function useEmailSettings() {
  return useSuspenseQuery(emailSettingsQueryOptions())
}

export async function updateEmailSettings(
  settings: EmailSettings,
): Promise<EmailSettings> {
  const res = await fetch('/api/v1/settings/email', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(
        res,
        `update email settings failed: ${res.status}`,
      ),
    )
  }
  return (await res.json()) as EmailSettings
}

export function useUpdateEmailSettings() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: updateEmailSettings,
    onSuccess: (updated) => {
      queryClient.setQueryData(emailSettingsKeys.all, updated)
      void queryClient.invalidateQueries({ queryKey: emailSettingsKeys.status })
      void queryClient.invalidateQueries({ queryKey: emailSettingsKeys.domain })
    },
  })
}

export interface EmailStatus {
  configured: boolean
  provider: string
  from?: string
  last_success_at?: string
  last_failure_at?: string
  last_error_kind?: string
  sent_24h: number
  failed_24h: number
}

export function emailStatusQueryOptions() {
  return queryOptions({
    queryKey: emailSettingsKeys.status,
    queryFn: async (): Promise<EmailStatus> => {
      const res = await fetch('/api/v1/settings/email/status')
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `email status failed: ${res.status}`),
        )
      }
      return (await res.json()) as EmailStatus
    },
    staleTime: 15_000,
  })
}

export function useEmailStatus() {
  return useQuery(emailStatusQueryOptions())
}

export type EmailFailureKind =
  | 'auth_failed'
  | 'sender_not_verified'
  | 'region_mismatch'
  | 'port_blocked'
  | 'tls_error'
  | 'rate_limited'
  | 'not_configured'
  | 'other'

// Thrown by a failed test send; code is the typed failure kind the UI turns into a Fix.
export class EmailTestError extends ApiError {
  readonly code: EmailFailureKind

  constructor(status: number, message: string, code: string) {
    super(status, message)
    this.name = 'EmailTestError'
    this.code = (code || 'other') as EmailFailureKind
  }
}

export interface EmailTestResult {
  to: string
  provider: string
  provider_id?: string
  message_id?: string
  attempts: number
}

export interface SendTestEmailInput {
  to: string
  /** Aim the send at one provider alone, with no failover. */
  providerId?: string
}

export async function sendTestEmail({
  to,
  providerId,
}: SendTestEmailInput): Promise<EmailTestResult> {
  const path = providerId
    ? `/api/v1/settings/email/providers/${encodeURIComponent(providerId)}/test`
    : '/api/v1/settings/email/test'
  const res = await fetch(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ to }),
  })
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as {
      error?: string
      code?: string
    } | null
    throw new EmailTestError(
      res.status,
      body?.error ?? `send test email failed: ${res.status}`,
      body?.code ?? '',
    )
  }
  return (await res.json()) as EmailTestResult
}

export function useSendTestEmail() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: sendTestEmail,
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: emailSettingsKeys.status })
      void queryClient.invalidateQueries({
        queryKey: ['email-settings', 'providers'],
      })
    },
  })
}

export interface EmailRecordCheck {
  type: 'spf' | 'dkim' | 'dmarc'
  name: string
  status: 'pass' | 'warn'
  code: string
  value?: string
  message: string
}

export interface EmailDomainCheck {
  domain: string
  provider: string
  records: EmailRecordCheck[]
  spf_include?: string
}

export async function fetchEmailDomainCheck(
  providerId?: string,
): Promise<EmailDomainCheck> {
  const query = providerId ? `?provider=${encodeURIComponent(providerId)}` : ''
  const res = await fetch(`/api/v1/settings/email/domain-check${query}`)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `domain check failed: ${res.status}`),
    )
  }
  return (await res.json()) as EmailDomainCheck
}

export function useEmailDomainCheck() {
  return useMutation({
    mutationFn: (providerId?: string) => fetchEmailDomainCheck(providerId),
  })
}

export interface EmailDelivery {
  id: string
  created_at: string
  provider: string
  to: string
  kind: string
  status: 'sent' | 'failed' | 'bounced'
  message_id: string
  error: string
  error_kind: string
  attempts: number
}

export interface EmailDeliveryFilters {
  status: '' | 'sent' | 'failed' | 'bounced'
  provider: string
  q: string
}

interface EmailDeliveryPage {
  deliveries: EmailDelivery[]
  next_before?: string
}

const DELIVERY_PAGE = 50

export function emailDeliveriesOptions(f: EmailDeliveryFilters) {
  return infiniteQueryOptions({
    queryKey: emailSettingsKeys.deliveries(f),
    initialPageParam: '',
    queryFn: async ({ pageParam }): Promise<EmailDeliveryPage> => {
      const params = new URLSearchParams({ limit: String(DELIVERY_PAGE) })
      if (f.status) params.set('status', f.status)
      if (f.provider) params.set('provider', f.provider)
      if (f.q) params.set('q', f.q)
      if (pageParam) params.set('before', pageParam)
      const res = await fetch(
        `/api/v1/settings/email/deliveries?${params.toString()}`,
      )
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `email log failed: ${res.status}`),
        )
      }
      return (await res.json()) as EmailDeliveryPage
    },
    getNextPageParam: (last) => last.next_before || undefined,
  })
}

export function useEmailDeliveries(f: EmailDeliveryFilters) {
  return useInfiniteQuery(emailDeliveriesOptions(f))
}
