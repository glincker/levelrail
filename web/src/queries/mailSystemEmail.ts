// System email through the instance mail server (internal/api/mail_system_email.go).
// No response carries the noreply credential; it is sealed server side.

import { useQuery } from '@tanstack/react-query'
import { mailSend } from './mailMailboxes'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface SystemEmailTest {
  to: string
  ok: boolean
  kind?: string
  message: string
  at: string
}

export interface SystemEmailDns {
  spf: string
  dkim: string
  dmarc: string
  pass: boolean
}

export interface SystemEmailState {
  enabled: boolean
  active: boolean
  domain?: string
  app?: string
  from_address?: string
  submission?: string
  tls_mode?: string
  certificate_pinned: boolean
  has_previous_provider: boolean
  last_test?: SystemEmailTest
  dns?: SystemEmailDns
  warning?: string
  demo?: boolean
  domains: string[]
  pin_mismatch?: boolean
  error?: string
}

export interface SystemEmailStep {
  id: string
  ok: boolean
  message: string
}

export interface SystemEmailResult {
  state: SystemEmailState
  steps: SystemEmailStep[]
}

export const systemEmailKey = ['mail', 'system-email'] as const
export const systemEmailStepUp = {
  action: 'manage',
  app: 'mail-system-email',
} as const
export const SYSTEM_EMAIL_ANCHOR = 'mail-system-email'

export const useSystemEmail = (enabled = true) =>
  useQuery({
    queryKey: systemEmailKey,
    queryFn: async (): Promise<SystemEmailState> => {
      const res = await fetch('/api/v1/mail/system-email')
      if (!res.ok) {
        throw new ApiError(
          res.status,
          await readErrorMessage(res, `system email failed: ${res.status}`),
        )
      }
      return (await res.json()) as SystemEmailState
    },
    enabled,
  })

export const enableSystemEmail = (
  domain: string,
  to: string,
  replaceExisting: boolean,
) =>
  mailSend<SystemEmailResult>('POST', '/api/v1/mail/system-email/enable', {
    domain,
    to: to || undefined,
    replace_existing: replaceExisting,
  })

export const systemEmailAction = (
  action: 'disable' | 'rotate' | 'test',
  to = '',
) =>
  mailSend<SystemEmailResult>('POST', `/api/v1/mail/system-email/${action}`, {
    to: to || undefined,
  })
