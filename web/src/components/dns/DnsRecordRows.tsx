import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CircleDashedGlassIcon,
  CircleXGlassIcon,
  ClockGlassIcon,
  CopyGlassIcon,
  ExternalLinkGlassIcon,
} from '@thesvg/ui-react'
import type { DnsStatusReport } from '@/queries/dnsStatus'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type {
  MailCheckReport,
  MailPlan,
  MailPlanChange,
} from '@/queries/mailDns'
import {
  groupChanges,
  rowStatus,
  splitPriority,
  tableText,
  zoneFileText,
  type RowStatus,
} from './dnsSetup'
import { useCopy } from './useCopy'

const TONE: Record<RowStatus, string> = {
  live: 'text-tone-success',
  waiting: 'text-muted-foreground',
  different: 'text-tone-warning',
  conflict: 'text-tone-danger',
  unknown: 'text-muted-foreground',
}

/** Never green for unknown: only a found, matching record gets the tick. */
export function StatusIcon({ status }: { status: RowStatus }) {
  const cls = TONE[status]
  if (status === 'live')
    return <CircleCheckGlassIcon className={cls} size="1.2em" />
  if (status === 'different')
    return <CircleAlertGlassIcon className={cls} size="1.2em" />
  if (status === 'conflict')
    return <CircleXGlassIcon className={cls} size="1.2em" />
  if (status === 'waiting')
    return <ClockGlassIcon className={cls} size="1.2em" />
  return <CircleDashedGlassIcon className={cls} size="1.2em" />
}

export function CopyIconButton({
  id,
  text,
  label,
  copied,
  onCopy,
}: {
  id: string
  text: string
  label: string
  copied: string | null
  onCopy: (id: string, text: string) => void
}) {
  const { t } = useTranslation('dnsSetup')
  const done = copied === id
  return (
    <button
      type="button"
      aria-label={label}
      onClick={() => onCopy(id, text)}
      className="inline-flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-xs text-muted-foreground hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
    >
      {done ? (
        <CheckGlassIcon size="1.1em" className="text-tone-success" />
      ) : (
        <CopyGlassIcon size="1.1em" />
      )}
      <span aria-live="polite">{done ? t('copied') : null}</span>
    </button>
  )
}

const rowKey = (c: MailPlanChange, i: number) =>
  `${c.set.name}-${c.set.type}-${i}`

/** Compact grouped status list. `phase` decides whether a missing record reads "Will add" or "Waiting". */
export function DnsRecordRows({
  kind,
  plan,
  report,
  status,
  phase,
  replaceKinds,
  onToggleReplace,
}: {
  kind: 'mail' | 'app'
  plan: MailPlan
  report?: MailCheckReport
  status?: DnsStatusReport | null
  phase: 'review' | 'verify'
  replaceKinds?: string[]
  onToggleReplace?: (kind: string) => void
}) {
  const { t } = useTranslation('dnsSetup')
  return (
    <div className="space-y-3">
      {groupChanges(kind, plan).map(({ group, changes }) => (
        <section key={group} aria-label={t(`group.${group}`)}>
          <h3 className="mb-1 text-xs font-medium text-muted-foreground">
            {t(`group.${group}`)}
          </h3>
          <ul className="divide-y divide-border rounded-md border border-border">
            {changes.map((c, i) => {
              const st = rowStatus(c, report, status)
              const staged = replaceKinds?.includes(c.kind) ?? false
              return (
                <li
                  key={rowKey(c, i)}
                  data-status={st}
                  className="flex flex-wrap items-center gap-x-2 gap-y-1 px-2 py-1.5 text-xs transition-colors motion-reduce:transition-none"
                >
                  <StatusIcon status={st} />
                  <Badge variant="muted" className="font-mono">
                    {c.set.type}
                  </Badge>
                  <span className="min-w-0 flex-1 break-all font-mono">
                    {c.set.name}
                  </span>
                  <span
                    className={cn('font-medium', TONE[st])}
                    data-testid="row-status"
                  >
                    {st === 'waiting' && phase === 'review'
                      ? t('status.willAdd')
                      : t(`status.${st}`)}
                  </span>
                  {c.before && (
                    <Badge variant="muted">{t('status.found')}</Badge>
                  )}
                  {st === 'different' && c.before && (
                    <div className="basis-full space-y-0.5 pl-6 text-muted-foreground">
                      <p className="break-all font-mono">
                        {t('foundValue', {
                          value: c.before.values.join(' | '),
                        })}
                      </p>
                      <p className="break-all font-mono">
                        {t('expectedValue', {
                          value: c.set.values.join(' | '),
                        })}
                      </p>
                      {onToggleReplace && phase === 'review' && (
                        <div className="flex gap-2 pt-0.5">
                          <Button
                            type="button"
                            size="sm"
                            variant={staged ? 'outline' : 'secondary'}
                            aria-pressed={!staged}
                            onClick={() => staged && onToggleReplace(c.kind)}
                          >
                            {t('keepMine')}
                          </Button>
                          <Button
                            type="button"
                            size="sm"
                            variant={staged ? 'secondary' : 'outline'}
                            aria-pressed={staged}
                            onClick={() => !staged && onToggleReplace(c.kind)}
                          >
                            {t('replace')}
                          </Button>
                        </div>
                      )}
                    </div>
                  )}
                  {st === 'conflict' && c.reason && (
                    <p className="basis-full pl-6 text-tone-danger">
                      {c.reason}
                    </p>
                  )}
                </li>
              )
            })}
          </ul>
        </section>
      ))}
    </div>
  )
}

/** The manual path: every record as a copyable row, plus zone-file and table copies. */
export function DnsManualRecords({
  plan,
  report,
  status: liveStatus,
  providerLabel,
  dashboardUrl,
}: {
  plan: MailPlan
  report?: MailCheckReport
  status?: DnsStatusReport | null
  providerLabel: string
  dashboardUrl?: string
}) {
  const { t } = useTranslation('dnsSetup')
  const { copied, copy } = useCopy()
  return (
    <div className="space-y-2" aria-label={t('manual.title')} role="group">
      <div className="flex flex-wrap items-center gap-2">
        <p className="min-w-0 flex-1 text-xs text-muted-foreground">
          {providerLabel
            ? t('manual.whereKnown', { provider: providerLabel })
            : t('manual.whereUnknown')}
        </p>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => copy('all', zoneFileText(plan))}
        >
          {copied === 'all' ? t('copied') : t('manual.copyAll')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => copy('table', tableText(plan))}
        >
          {copied === 'table' ? t('copied') : t('manual.copyTable')}
        </Button>
        {dashboardUrl && (
          <a
            href={dashboardUrl}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 text-xs underline underline-offset-2"
          >
            {t('openIn', { provider: providerLabel || t('manual.dashboard') })}
            <ExternalLinkGlassIcon size="1.1em" />
          </a>
        )}
      </div>
      <ul className="space-y-1.5">
        {plan.changes.map((c, i) => {
          const st = rowStatus(c, report, liveStatus)
          return c.set.values.map((v, j) => {
            const { priority, value } = splitPriority(c.set, v)
            const id = `${rowKey(c, i)}-${j}`
            return (
              <li
                key={id}
                className="rounded-md border border-border p-2 text-xs"
              >
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant="muted" className="font-mono">
                    {c.set.type}
                  </Badge>
                  <span className="min-w-0 flex-1 break-all font-mono">
                    {c.set.name}
                  </span>
                  <span className="flex items-center gap-1 text-muted-foreground">
                    <StatusIcon status={st} />
                    {t(`status.${st}`)}
                  </span>
                </div>
                <div className="mt-1 flex items-start gap-2">
                  <code className="min-w-0 flex-1 break-all rounded bg-muted px-1.5 py-1 font-mono">
                    {value}
                  </code>
                  <CopyIconButton
                    id={id}
                    text={value}
                    label={t('manual.copyValue', { name: c.set.name })}
                    copied={copied}
                    onCopy={copy}
                  />
                </div>
                <p className="mt-1 text-muted-foreground">
                  {t('manual.ttl', { ttl: c.set.ttl })}
                  {priority ? ` · ${t('manual.priority', { priority })}` : ''}
                </p>
              </li>
            )
          })
        })}
      </ul>
    </div>
  )
}
