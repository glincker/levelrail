import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowsLeftRightIcon,
  ArrowUUpLeftIcon,
  ChartLineUpIcon,
  EyeIcon,
  FlagCheckeredIcon,
  GitBranchIcon,
  HammerIcon,
  HeartbeatIcon,
  ListChecksIcon,
  TerminalWindowIcon,
  UploadSimpleIcon,
  WebhooksLogoIcon,
} from '@phosphor-icons/react/dist/ssr'
import { StatusPill } from '@/components/kit'
import { TONE } from '@/components/kit/tone'
import { cn } from '@/lib/utils'
import {
  codeKey,
  formatMs,
  groupRows,
  statusTone,
} from '../../lib/deployEvents'
import type { DeployEvent, DeployEventKind } from '../../types/deployEvents'
import { ImpactCard } from './ImpactCard'

const KIND_ICON: Record<DeployEventKind, ReactNode> = {
  trigger: <GitBranchIcon />,
  build: <HammerIcon />,
  log: <TerminalWindowIcon />,
  image_push: <UploadSimpleIcon />,
  hook: <WebhooksLogoIcon />,
  step: <ListChecksIcon />,
  readiness: <HeartbeatIcon />,
  cutover: <ArrowsLeftRightIcon />,
  watch: <EyeIcon />,
  rollback: <ArrowUUpLeftIcon />,
  outcome: <FlagCheckeredIcon />,
  traffic_impact: <ChartLineUpIcon />,
}

function clock(at: string): string {
  const d = new Date(at)
  return Number.isNaN(d.getTime())
    ? at
    : d.toLocaleTimeString(undefined, {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false,
      })
}

function LogGroup({ lines }: { lines: DeployEvent[] }) {
  const { t } = useTranslation('deployEvents')
  return (
    <details className="group rounded-lg border bg-muted/20">
      <summary className="cursor-pointer rounded-lg px-3 py-2 text-sm font-medium outline-none select-none focus-visible:ring-2 focus-visible:ring-ring/60">
        {t('logs.summary', { count: lines.length })}
      </summary>
      <pre
        tabIndex={0}
        aria-label={t('logs.region')}
        className="max-h-72 overflow-auto border-t px-3 py-2 font-mono text-xs leading-5 whitespace-pre-wrap outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
      >
        {lines.map((l) => (
          <span
            key={l.id}
            className={cn(
              'block',
              l.stream === 'stderr' && 'text-tone-warning',
            )}
          >
            {l.detail}
          </span>
        ))}
      </pre>
    </details>
  )
}

function EventRow({ event }: { event: DeployEvent }) {
  const { t } = useTranslation('deployEvents')
  const tone = TONE[statusTone(event.status)]
  const title = t(`codes.${codeKey(event.code)}` as 'codes.push', {
    defaultValue: event.title,
  })
  const trig = event.trigger
  return (
    <>
      <span
        aria-hidden="true"
        className={cn(
          'relative z-10 flex size-8 shrink-0 items-center justify-center rounded-full border [&_svg]:size-4',
          tone.soft,
          tone.border,
          tone.text,
        )}
      >
        {KIND_ICON[event.kind]}
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="text-sm font-medium">{title}</span>
          <StatusPill
            size="sm"
            tone={statusTone(event.status)}
            label={t(`status.${event.status}`)}
            live={event.status === 'running'}
          />
          <time
            dateTime={event.at}
            className="text-xs text-muted-foreground tabular-nums"
          >
            {clock(event.at)}
          </time>
          {event.duration_ms ? (
            <span className="text-xs text-muted-foreground tabular-nums">
              {formatMs(event.duration_ms)}
            </span>
          ) : null}
        </div>
        {event.detail && (
          <p className="mt-0.5 text-sm break-words text-muted-foreground">
            {event.detail}
          </p>
        )}
        {event.reason && (
          <p className="mt-0.5 font-mono text-xs text-muted-foreground">
            {event.reason}
          </p>
        )}
        {trig?.rerun_of && (
          <p className="mt-0.5 text-xs text-muted-foreground">
            {t('trigger.rerun', { id: trig.rerun_of })}
          </p>
        )}
        {event.impact && <ImpactCard impact={event.impact} />}
      </div>
    </>
  )
}

export interface EventListProps {
  events: DeployEvent[]
  label?: string
  showLogs?: boolean
}

export function EventList({ events, label, showLogs = true }: EventListProps) {
  const { t } = useTranslation('deployEvents')
  const rows = groupRows(
    showLogs ? events : events.filter((e) => e.kind !== 'log'),
  )
  return (
    <ol aria-label={label ?? t('list.label')} className="flex flex-col">
      {rows.map((row, i) => (
        <li
          key={row.kind === 'event' ? row.event.id : row.id}
          className="relative flex gap-3 pb-4 last:pb-0"
        >
          {i < rows.length - 1 && (
            <span
              aria-hidden="true"
              className="absolute top-8 bottom-0 left-4 w-px -translate-x-1/2 bg-border"
            />
          )}
          {row.kind === 'event' ? (
            <EventRow event={row.event} />
          ) : (
            <>
              <span
                aria-hidden="true"
                className="relative z-10 flex size-8 shrink-0 items-center justify-center rounded-full border bg-muted text-muted-foreground [&_svg]:size-4"
              >
                {KIND_ICON.log}
              </span>
              <div className="min-w-0 flex-1">
                <LogGroup lines={row.lines} />
              </div>
            </>
          )}
        </li>
      ))}
    </ol>
  )
}
