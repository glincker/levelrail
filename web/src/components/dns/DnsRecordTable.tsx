import { useState, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  ChevronDownGlassIcon,
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CirclePlusGlassIcon,
  CircleQuestionMarkGlassIcon,
  ClockGlassIcon,
  CopyGlassIcon,
  ShieldAlertGlassIcon,
} from '@thesvg/ui-react'
import type { DnsStatusReport } from '@/queries/dnsStatus'
import { cn } from '@/lib/utils'
import type { MailCheckReport, MailPlan } from '@/queries/mailDns'
import {
  groupChanges,
  rowStatus,
  splitPriority,
  type RowStatus,
} from './dnsSetup'
import { useCopy } from './useCopy'

export type ChipStatus = RowStatus | 'willAdd'

// Icon shape differs per state so the chip never relies on colour alone.
const TONE: Record<ChipStatus, string> = {
  live: 'text-tone-success',
  willAdd: 'text-foreground',
  waiting: 'text-muted-foreground',
  different: 'text-tone-warning',
  conflict: 'text-tone-warning',
  unknown: 'text-muted-foreground',
}

const ICONS: Record<ChipStatus, typeof CheckGlassIcon> = {
  live: CircleCheckGlassIcon,
  willAdd: CirclePlusGlassIcon,
  waiting: ClockGlassIcon,
  different: CircleAlertGlassIcon,
  conflict: ShieldAlertGlassIcon,
  unknown: CircleQuestionMarkGlassIcon,
}

/** Never green for unknown: only a found, matching record gets the tick. */
export function StatusIcon({ status }: { status: ChipStatus }) {
  const Icon = ICONS[status]
  return <Icon className={TONE[status]} size="1.2em" />
}

// Long values stay one truncated line; the chevron reveals the full text.
const EXPAND_CHARS = Math.max(
  1,
  Number(import.meta.env.VITE_DNS_EXPAND_CHARS ?? 48),
)

// Mobile is a 2-line grid; from sm up every cell gets its own column.
const ROW_GRID =
  'grid grid-cols-[auto_auto_minmax(0,1fr)_auto] items-center gap-x-2 gap-y-0.5 sm:grid-cols-[7rem_3.25rem_minmax(0,13rem)_minmax(0,1fr)_3rem_2rem]'
const C_STATUS = 'col-start-1 row-start-1 sm:col-auto sm:row-auto'
const C_NAME =
  'col-span-3 col-start-2 row-start-1 min-w-0 sm:col-auto sm:row-auto sm:col-span-1'
const C_TYPE = 'col-start-1 row-start-2 sm:col-auto sm:row-auto'
const C_VALUE =
  'col-span-2 col-start-2 row-start-2 min-w-0 sm:col-auto sm:row-auto sm:col-span-1'
const C_COPY = 'col-start-4 row-start-2 sm:col-auto sm:row-auto'

export function CopyValueButton({
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
      data-copy
      onClick={() => onCopy(id, text)}
      className={cn(
        'inline-flex size-8 shrink-0 items-center justify-center gap-1 rounded text-xs text-muted-foreground hover:bg-muted focus-visible:opacity-100 focus-visible:outline-2 focus-visible:outline-ring sm:opacity-0 sm:group-focus-within:opacity-100 sm:group-hover:opacity-100 [@media(hover:none)]:opacity-100',
        done && 'w-auto px-1.5 opacity-100 sm:opacity-100',
      )}
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

/** One compact table for review and verify: status, type, name, value, TTL. */
export function DnsRecordTable({
  kind,
  plan,
  report,
  status,
  phase,
  replaceKinds,
  onToggleReplace,
  checkedAgo,
}: {
  kind: 'mail' | 'app'
  plan: MailPlan
  report?: MailCheckReport
  status?: DnsStatusReport | null
  phase: 'review' | 'verify'
  replaceKinds?: string[]
  onToggleReplace?: (kind: string) => void
  /** Seconds since the last check, for the Waiting tooltip. */
  checkedAgo?: number | null
}) {
  const { t } = useTranslation('dnsSetup')
  const { copied, copy } = useCopy()
  const [open, setOpen] = useState<string[]>([])
  const toggleOpen = (id: string) =>
    setOpen((cur) =>
      cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id],
    )

  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
    const rows = Array.from(
      e.currentTarget.querySelectorAll<HTMLElement>('[data-row]'),
    )
    const at = rows.findIndex((r) => r.contains(document.activeElement))
    const next = rows[at + (e.key === 'ArrowDown' ? 1 : -1)]
    if (!next) return
    e.preventDefault()
    next.focus()
  }

  const groups = groupChanges(kind, plan)
  return (
    <div
      role="table"
      aria-label={t('table.label')}
      onKeyDown={onKey}
      className="overflow-hidden rounded-md border border-border"
    >
      <div role="row" className={cn(ROW_GRID, 'sr-only')}>
        {(['status', 'type', 'name', 'value', 'ttl'] as const).map((c) => (
          <span key={c} role="columnheader">
            {t(`table.${c}`)}
          </span>
        ))}
      </div>
      {groups.map(({ group, changes }) => (
        <div key={group} role="rowgroup" aria-label={t(`group.${group}`)}>
          <div
            role="row"
            className="flex items-center gap-1.5 border-b border-border bg-muted/50 px-2 py-1 text-xs font-medium uppercase tracking-wide text-muted-foreground"
          >
            <span role="rowheader">{t(`group.${group}`)}</span>
            <span aria-hidden>{changes.length}</span>
          </div>
          {changes.flatMap((c, i) => {
            const st = rowStatus(c, report, status)
            const chip: ChipStatus =
              st === 'waiting' && phase === 'review' ? 'willAdd' : st
            const staged = replaceKinds?.includes(c.kind) ?? false
            return c.set.values.map((v, j) => {
              const { priority, value } = splitPriority(c.set, v)
              const id = `${c.set.name}-${c.set.type}-${i}-${j}`
              const long = value.length > EXPAND_CHARS
              const expanded = open.includes(id)
              const tabIndex =
                group === groups[0]?.group && i === 0 && j === 0 ? 0 : -1
              const tip =
                chip === 'waiting' && checkedAgo != null
                  ? t('verify.checkedAgo', { seconds: checkedAgo })
                  : undefined
              return (
                <div
                  key={id}
                  role="row"
                  data-row
                  data-status={st}
                  tabIndex={tabIndex}
                  onKeyDown={(e) => {
                    if (
                      e.key === 'Enter' &&
                      e.target === e.currentTarget &&
                      !e.metaKey &&
                      !e.ctrlKey
                    ) {
                      e.preventDefault()
                      copy(id, value)
                    }
                  }}
                  className="group min-h-11 border-b border-border px-2 py-1.5 text-sm last:border-b-0 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring"
                >
                  <div role="none" className={ROW_GRID}>
                    <span
                      role="cell"
                      title={tip}
                      className={cn(
                        C_STATUS,
                        'inline-flex items-center gap-1 text-xs font-medium',
                        TONE[chip],
                      )}
                    >
                      <StatusIcon status={chip} />
                      <span data-testid="row-status">
                        {t(`status.${chip}`)}
                      </span>
                    </span>
                    <span
                      role="cell"
                      className={cn(C_NAME, 'truncate font-mono text-[13px]')}
                      title={c.set.name}
                    >
                      {c.set.name}
                      <span className="text-xs text-muted-foreground sm:hidden">
                        {` · ${t('manual.ttl', { ttl: c.set.ttl })}`}
                        {priority
                          ? ` · ${t('manual.priority', { priority })}`
                          : ''}
                      </span>
                    </span>
                    <span
                      role="cell"
                      className={cn(
                        C_TYPE,
                        'font-mono text-xs text-muted-foreground',
                      )}
                    >
                      {c.set.type}
                    </span>
                    <span
                      role="cell"
                      className={cn(C_VALUE, 'flex items-center gap-1')}
                    >
                      {priority && (
                        <span className="hidden shrink-0 text-xs text-muted-foreground sm:inline">
                          {priority}
                        </span>
                      )}
                      <button
                        type="button"
                        title={value}
                        aria-label={t('manual.copyValue', { name: c.set.name })}
                        onClick={() => copy(id, value)}
                        className="min-w-0 flex-1 truncate rounded text-left font-mono text-[13px] focus-visible:outline-2 focus-visible:outline-ring"
                      >
                        {value}
                      </button>
                      {long && (
                        <button
                          type="button"
                          aria-expanded={expanded}
                          aria-label={t(
                            expanded ? 'table.collapse' : 'table.expand',
                          )}
                          onClick={() => toggleOpen(id)}
                          className="inline-flex size-8 shrink-0 items-center justify-center rounded text-muted-foreground hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring"
                        >
                          <ChevronDownGlassIcon
                            size="1.1em"
                            className={cn(
                              'transition-transform motion-reduce:transition-none',
                              expanded && 'rotate-180',
                            )}
                          />
                        </button>
                      )}
                    </span>
                    <span
                      role="cell"
                      className="hidden text-xs text-muted-foreground sm:block"
                    >
                      {c.set.ttl}
                    </span>
                    <span role="cell" className={C_COPY}>
                      <CopyValueButton
                        id={id}
                        text={value}
                        label={t('manual.copyValue', { name: c.set.name })}
                        copied={copied}
                        onCopy={copy}
                      />
                    </span>
                  </div>
                  {expanded && (
                    <code className="mt-1 block whitespace-pre-wrap break-all rounded bg-muted p-2 font-mono text-[13px]">
                      {value}
                    </code>
                  )}
                  {j === 0 && c.before && (
                    <div className="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-1 pl-0 text-xs text-muted-foreground sm:pl-[7.5rem]">
                      <span className="min-w-0 break-all font-mono">
                        {t('foundValue', {
                          value: c.before.values.join(' | '),
                        })}
                      </span>
                      {st === 'different' &&
                        onToggleReplace &&
                        phase === 'review' && (
                          <span
                            role="group"
                            aria-label={t('table.existing')}
                            className="inline-flex overflow-hidden rounded border border-border"
                          >
                            <button
                              type="button"
                              aria-pressed={!staged}
                              onClick={() => staged && onToggleReplace(c.kind)}
                              className={cn(
                                'px-2 py-1 focus-visible:outline-2 focus-visible:outline-ring',
                                !staged
                                  ? 'bg-muted text-foreground'
                                  : 'hover:bg-muted',
                              )}
                            >
                              {t('keepMine')}
                            </button>
                            <button
                              type="button"
                              aria-pressed={staged}
                              onClick={() => !staged && onToggleReplace(c.kind)}
                              className={cn(
                                'border-l border-border px-2 py-1 focus-visible:outline-2 focus-visible:outline-ring',
                                staged
                                  ? 'bg-muted text-foreground'
                                  : 'hover:bg-muted',
                              )}
                            >
                              {t('replace')}
                            </button>
                          </span>
                        )}
                    </div>
                  )}
                  {j === 0 && st === 'conflict' && c.reason && (
                    <p className="mt-0.5 text-xs text-tone-warning sm:pl-[7.5rem]">
                      {c.reason}
                    </p>
                  )}
                </div>
              )
            })
          })}
        </div>
      ))}
    </div>
  )
}
