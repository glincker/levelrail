// Mail mailbox, alias, queue and domain APIs (internal/api/mail_mailboxes.go).
// Generated passwords are returned to the caller only: never cached, never persisted.

import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { EnvApiError } from './envProtection'

export interface MailDomain {
  domain: string
  service: string
  team_id: string
  created_at: string
}

export interface Mailbox {
  address: string
  domain: string
  localpart: string
  team_id: string
  linked_user_id?: string
  suspended: boolean
  suspend_pending: boolean
  created_at: string
}

export interface MailAlias {
  address: string
  targets: string[]
}

export interface MailQueueEntry {
  id: number
  queued: string
  hold: boolean
  sender_account: string
  recipient_domain: string
  attempts: number
  next_attempt: string
  last_error?: string
}

export interface MailQueue {
  app: string
  queued: number
  entries: MailQueueEntry[]
}

export interface MailboxSecret {
  mailbox: Mailbox
  password: string
  notice: string
}

export const mailKeys = {
  domains: ['mail', 'domains'] as const,
  mailboxes: ['mail', 'mailboxes'] as const,
  aliases: ['mail', 'aliases'] as const,
  queue: ['mail', 'queue'] as const,
}

export const mailStepUpScope = {
  password: (address: string) =>
    ({ action: 'manage', app: `mail-password:${address}` }) as const,
  remove: (address: string) =>
    ({ action: 'manage', app: `mail-delete:${address}` }) as const,
  release: (app: string) =>
    ({ action: 'manage', app: `mail-queue-release:${app}` }) as const,
  domain: (domain: string) =>
    ({ action: 'manage', app: `mail-domain:${domain}` }) as const,
}

const enc = encodeURIComponent

async function send<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(path, {
    method,
    headers:
      body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
    cache: 'no-store',
  })
  if (!res.ok) {
    const err = (await res.json().catch(() => null)) as {
      error?: string
      code?: string
    } | null
    // EnvApiError carries step_up_required so useStepUpGate can catch it.
    throw new EnvApiError(
      res.status,
      err?.error ?? `mail request failed: ${res.status}`,
      err?.code ?? '',
    )
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

/** mailSend is the shared mail write helper; errors keep the step-up code. */
export const mailSend = send

async function read<T>(path: string): Promise<T> {
  const res = await fetch(path)
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `mail request failed: ${res.status}`),
    )
  }
  return (await res.json()) as T
}

export const useMailDomains = (enabled = true) =>
  useQuery({
    queryKey: mailKeys.domains,
    queryFn: () => read<MailDomain[]>('/api/v1/mail/domains'),
    enabled,
  })

export const useMailboxes = (enabled = true) =>
  useQuery({
    queryKey: mailKeys.mailboxes,
    queryFn: () => read<Mailbox[]>('/api/v1/mail/mailboxes'),
    enabled,
  })

// The aliases route is per domain, so the hook takes the claimed domain list.
export const useMailAliases = (domains: string[]) =>
  useQuery({
    queryKey: [...mailKeys.aliases, domains],
    queryFn: async () =>
      (
        await Promise.all(
          domains.map((d) =>
            read<MailAlias[]>(`/api/v1/mail/aliases?domain=${enc(d)}`),
          ),
        )
      ).flat(),
    enabled: domains.length > 0,
  })

// A 403 or 404 means the caller is not an instance member: the panel hides.
export const useMailQueue = (enabled = true) =>
  useQuery({
    queryKey: mailKeys.queue,
    queryFn: () => read<MailQueue>('/api/v1/mail/queue'),
    enabled,
    retry: false,
  })

export const isHiddenQueueError = (e: unknown): boolean =>
  e instanceof ApiError && (e.status === 403 || e.status === 404)

export const createMailbox = (address: string, linkedUserId: string) =>
  send<MailboxSecret>('POST', '/api/v1/mail/mailboxes', {
    address,
    linked_user_id: linkedUserId || undefined,
  })

export const resetMailboxPassword = (address: string) =>
  send<MailboxSecret>(
    'POST',
    `/api/v1/mail/mailboxes/${enc(address)}/password`,
    {},
  )

export const setMailboxSuspended = (address: string, suspend: boolean) =>
  send<Mailbox>(
    'POST',
    `/api/v1/mail/mailboxes/${enc(address)}/${suspend ? 'suspend' : 'resume'}`,
  )

export const deleteMailbox = (address: string) =>
  send<void>('DELETE', `/api/v1/mail/mailboxes/${enc(address)}`, {
    confirm: address,
    delete_mail: false,
  })

export const addMailAlias = (address: string, targets: string[]) =>
  send<MailAlias>('POST', '/api/v1/mail/aliases', { address, targets })

export const removeMailAlias = (address: string) =>
  send<void>('DELETE', `/api/v1/mail/aliases/${enc(address)}`)

export const setQueueHold = (hold: boolean, app: string, ids: number[]) =>
  send<{ changed: number }>(
    'POST',
    `/api/v1/mail/queue/${hold ? 'hold' : 'release'}`,
    { app, ids },
  )

export const claimMailDomain = (domain: string, teamId: string) =>
  send<MailDomain>('POST', '/api/v1/mail/domains', {
    domain,
    team_id: teamId,
  })

export function useInvalidateMail() {
  const qc = useQueryClient()
  return () => qc.invalidateQueries({ queryKey: ['mail'] })
}
