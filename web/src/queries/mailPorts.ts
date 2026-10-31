// GET /api/v1/mail/ports and POST /api/v1/mail/ports/open
// (internal/api/mail_ports.go): per-port bind and firewall state of the
// four mail ports, and the one-step open.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { EnvApiError } from './envProtection'

export const MAIL_PORTS_STEP_UP_APP = 'mail-ports'

export type MailPortBind = 'private' | 'public' | 'not_published'
export type MailPortFirewall =
  'allowed' | 'blocked' | 'not_filtering' | 'unknown'

export interface MailPortRow {
  port: number
  purpose: string
  published: boolean
  bind: MailPortBind
  firewall: MailPortFirewall
}

export interface MailPorts {
  app: string
  installed: boolean
  ports: MailPortRow[]
  web_listener: string
  port_25_note: string
  warning: string
  next_step: string
  bind_scope_note: string
}

export interface MailPortsOpenResult {
  app: string
  changed: number[]
  result: string
  warning: string
  pending?: number[]
  message: string
}

export const mailPortsKey = ['mail', 'ports'] as const

export function mailPortsStepUpScope(app: string) {
  return { action: 'manage', app: `${MAIL_PORTS_STEP_UP_APP}:${app}` } as const
}

export async function fetchMailPorts(): Promise<MailPorts> {
  const res = await fetch('/api/v1/mail/ports')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `mail ports failed: ${res.status}`),
    )
  }
  return (await res.json()) as MailPorts
}

// Throws EnvApiError so useStepUpGate can catch step_up_required.
export async function openMailPorts(app: string): Promise<MailPortsOpenResult> {
  const res = await fetch('/api/v1/mail/ports/open', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ app, confirm: true }),
    cache: 'no-store',
  })
  const body = (await res.json().catch(() => null)) as
    (Partial<MailPortsOpenResult> & { error?: string; code?: string }) | null
  if (!res.ok) {
    throw new EnvApiError(
      res.status,
      body?.error ?? body?.message ?? `open failed: ${res.status}`,
      body?.code ?? '',
    )
  }
  return body as MailPortsOpenResult
}

export function useMailPorts() {
  return useQuery({ queryKey: mailPortsKey, queryFn: fetchMailPorts })
}

export function useOpenMailPorts() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: openMailPorts,
    onSettled: () => qc.invalidateQueries({ queryKey: mailPortsKey }),
  })
}
