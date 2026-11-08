// GET /api/v1/mail/cert and POST /api/v1/mail/cert/obtain
// (internal/api/mail_cert.go): the certificate the mail ports serve.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export interface MailCert {
  app: string
  hostname: string
  source: string
  served: boolean
  subject?: string
  issuer?: string
  not_after?: string
  days_left: number
  self_signed: boolean
  serial?: string
  sha256?: string
  probe_error?: string
  restart_window: string
  next_step: string
}

export const mailCertKey = ['mail', 'cert'] as const

export async function fetchMailCert(): Promise<MailCert> {
  const res = await fetch('/api/v1/mail/cert')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `mail certificate failed: ${res.status}`),
    )
  }
  return (await res.json()) as MailCert
}

export async function obtainMailCert(app: string): Promise<void> {
  const res = await fetch('/api/v1/mail/cert/obtain', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ app }),
  })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `obtain certificate failed: ${res.status}`),
    )
  }
}

export function useMailCert(enabled = true) {
  return useQuery({ queryKey: mailCertKey, queryFn: fetchMailCert, enabled })
}

export function useObtainMailCert() {
  const qc = useQueryClient()
  return useMutation<void, ApiError, string>({
    mutationFn: obtainMailCert,
    onSettled: () => qc.invalidateQueries({ queryKey: mailCertKey }),
  })
}
