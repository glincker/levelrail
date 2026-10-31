import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CircleDashedGlassIcon,
  CircleXGlassIcon,
} from '@thesvg/ui-react'
import type {
  MailCheckItem,
  MailCheckReport,
  MailCheckStatus,
} from '@/queries/mailDns'
import { isMailUp, type MailStatus } from '@/queries/mailStatus'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'

const STATUS_TONE: Record<MailCheckStatus, string> = {
  pass: 'text-tone-success',
  warn: 'text-tone-warning',
  fail: 'text-tone-danger',
  not_tested: 'text-muted-foreground',
}

export function StatusGlyph({ status }: { status: MailCheckStatus }) {
  const cls = STATUS_TONE[status]
  if (status === 'pass')
    return <CircleCheckGlassIcon className={cls} size="1.2em" />
  if (status === 'warn')
    return <CircleAlertGlassIcon className={cls} size="1.2em" />
  if (status === 'fail')
    return <CircleXGlassIcon className={cls} size="1.2em" />
  return <CircleDashedGlassIcon className={cls} size="1.2em" />
}

// Checklist rows in setup order. Server and PTR have no probe yet, so they
// stay "not checked" instead of guessing.
const ROWS = [
  { key: 'server', check: null },
  { key: 'dns', check: 'mx' },
  { key: 'spf', check: 'spf' },
  { key: 'dkim', check: 'dkim' },
  { key: 'dmarc', check: 'dmarc' },
  { key: 'mta_sts', check: 'mta_sts' },
  { key: 'ptr', check: null },
  { key: 'port25', check: 'port25' },
] as const

function findItem(
  report: MailCheckReport | undefined,
  id: string | null,
): MailCheckItem | undefined {
  return id ? report?.items.find((i) => i.id === id) : undefined
}

/** MailChecklist shows each setup step with its last checked state; unknown steps say so. */
export function MailChecklist({
  report,
  status,
}: {
  report: MailCheckReport | undefined
  status?: MailStatus
}) {
  const { t } = useTranslation('mailServer')
  return (
    <section
      aria-label={t('overview.title')}
      className="rounded-lg border border-border bg-card p-3"
    >
      <h2 className="mb-2 text-sm font-medium">{t('overview.title')}</h2>
      {!report && (
        <p className="mb-2 text-xs text-muted-foreground">
          {t('overview.hint')}
        </p>
      )}
      <ul
        aria-label={t('overview.label', { domain: report?.domain ?? '' })}
        className="grid gap-x-4 gap-y-1.5 sm:grid-cols-2 xl:grid-cols-4"
      >
        {ROWS.map((row) => {
          const item = findItem(report, row.check)
          let rowStatus: MailCheckStatus = item?.status ?? 'not_tested'
          let serverDetail: string | undefined
          if (row.key === 'server' && status) {
            if (isMailUp(status)) {
              rowStatus = 'pass'
              serverDetail = t('server.done')
            } else if (status.installed) {
              rowStatus = 'warn'
              serverDetail = status.next_step || t('server.stopped')
            }
          }
          const detail =
            serverDetail ??
            (item && item.status !== 'not_tested'
              ? item.message
              : row.key === 'server' || row.key === 'ptr'
                ? t(`unknown.${row.key}`)
                : (item?.message ?? t('status.not_tested')))
          return (
            <li key={row.key} className="flex items-start gap-2 text-sm">
              <span className="mt-0.5 shrink-0">
                <StatusGlyph status={rowStatus} />
              </span>
              <span className="min-w-0">
                <span className="font-medium">{t(`items.${row.key}`)}</span>
                <span className="sr-only"> {t(`status.${rowStatus}`)}</span>
                <span
                  className="block truncate text-xs text-muted-foreground"
                  title={detail}
                >
                  {rowStatus === 'not_tested' ? t('status.not_tested') : detail}
                </span>
                {row.key === 'server' && status && !status.installed && (
                  <Link
                    to="/templates"
                    search={{ deploy: 'mail-mox' }}
                    className={cn(
                      buttonVariants({ variant: 'outline', size: 'sm' }),
                      'mt-1',
                    )}
                  >
                    {t('server.installAction')}
                  </Link>
                )}
              </span>
            </li>
          )
        })}
      </ul>
    </section>
  )
}
