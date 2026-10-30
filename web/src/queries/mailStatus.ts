// GET /api/v1/mail/status (internal/api/mail_status.go): one cheap read of
// the mail app state, polled until the server is fully up.

import { useQuery } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

export const MAIL_STATUS_POLL_MS = 15000

export interface MailStatus {
  app?: string
  installed: boolean
  container_state: string
  admin_reachable: boolean
  tls: string
  next_step: string
}

export async function fetchMailStatus(): Promise<MailStatus> {
  const res = await fetch('/api/v1/mail/status')
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `mail status failed: ${res.status}`),
    )
  }
  return (await res.json()) as MailStatus
}

export const isMailUp = (s: MailStatus | undefined): boolean =>
  !!s && s.installed && s.container_state === 'running' && s.admin_reachable

export function useMailStatus() {
  return useQuery({
    queryKey: ['mail', 'status'],
    queryFn: fetchMailStatus,
    refetchInterval: (q) =>
      isMailUp(q.state.data) ? false : MAIL_STATUS_POLL_MS,
  })
}
