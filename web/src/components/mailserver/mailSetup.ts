import type { AppUrls } from '../../queries/appUrls'
import type { MailCheckReport } from '@/queries/mailDns'
import { isMailUp, type MailStatus } from '@/queries/mailStatus'

type StepKey = 'install' | 'fix' | 'wait' | 'dns' | 'verify'

/** What exists already; lets the overview settle without a DNS report. */
export interface MailFacts {
  domains: number
  mailboxes: number
  backups: number
}

export function nextStepOf(
  s: MailStatus,
  report: MailCheckReport | undefined,
  facts?: MailFacts,
): StepKey | null {
  if (!s.installed) return 'install'
  if (s.container_state === 'missing' || s.container_state === 'stopped')
    return 'fix'
  if (s.container_state === 'running' && !s.admin_reachable) return 'wait'
  if (!isMailUp(s)) return null
  if (!report) {
    // DNS stays its own "not checked" chip; it never counts as passing.
    const ready =
      facts && facts.domains > 0 && facts.mailboxes > 0 && facts.backups > 0
    return ready ? null : 'dns'
  }
  return report.items.some((i) => i.status !== 'pass') ? 'verify' : null
}

/** Mox serves its own webmail under /webmail/ on the web listener; we only link to it. */
export function webmailUrl(urls: AppUrls | undefined): string | undefined {
  const hit =
    urls?.addresses?.find((a) => a.kind === 'domain' && a.url) ??
    urls?.addresses?.find((a) => a.kind === 'public' && a.url)
  if (!hit?.url) return undefined
  return `${hit.url.replace(/\/+$/, '')}/webmail/`
}
