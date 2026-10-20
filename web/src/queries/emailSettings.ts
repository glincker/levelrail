// GET/PUT /api/v1/settings/email
// (internal/api/email_settings.go's emailSettingsResource).

import {
  queryOptions,
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const emailSettingsKeys = {
  all: ['email-settings'] as const,
}

// EmailSettings mirrors emailSettingsResource exactly. smtp_password/
// ses_secret_access_key/resend_api_key are write-only: only ever sent,
// never present in a GET response (the *_set flags report presence
// instead).
export interface EmailSettings {
  backend: '' | 'smtp' | 'ses' | 'resend'
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
  resend_from?: string
  resend_api_key?: string
  resend_api_key_set?: boolean
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
    },
  })
}

export async function sendTestEmail(to: string): Promise<void> {
  const res = await fetch('/api/v1/settings/email/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ to }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `send test email failed: ${res.status}`),
    )
  }
}

export function useSendTestEmail() {
  return useMutation({ mutationFn: sendTestEmail })
}
