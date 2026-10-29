import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ClockGlassIcon,
  CommitGraphGlassIcon,
  LayersGlassIcon,
  RefreshCwGlassIcon,
  RocketGlassIcon,
  RotateCcwGlassIcon,
  UserGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import { RelativeTime, StatusPill } from '@/components/kit'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatDurationMs } from '../../lib/deployDuration'
import {
  mergeAppHistory,
  type HistoryEntry,
  type HistoryKind,
} from '../../lib/appDeployHistory'
import type { DeployAttempt } from '../../types/deployAttempt'
import type { TimelineEntry } from '../../queries/appTimeline'

const KIND_ICON: Record<HistoryKind, UiIcon> = {
  deploy: RocketGlassIcon,
  rollback: RotateCcwGlassIcon,
  restart: RefreshCwGlassIcon,
}

export interface AppHistoryTimelineProps {
  appName: string
  attempts: readonly DeployAttempt[]
  activity: readonly TimelineEntry[]
  /** Stack managing this app, shown as the source of its desired state. */
  managedByStack?: string
}

// One compact timeline of deploys, restarts and rollbacks for an app.
export function AppHistoryTimeline({
  appName,
  attempts,
  activity,
  managedByStack,
}: Readonly<AppHistoryTimelineProps>) {
  const { t } = useTranslation('deployTimeline')
  const entries = mergeAppHistory(attempts, activity)
  if (entries.length === 0) return null

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2 space-y-0">
        <CardTitle>{t('appHistory.title')}</CardTitle>
        {managedByStack ? (
          <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
            <LayersGlassIcon className="size-5" aria-hidden="true" />
            {t('appHistory.managedBy', { stack: managedByStack })}
          </span>
        ) : null}
      </CardHeader>
      <CardContent>
        <ol
          aria-label={t('appHistory.title')}
          className="divide-y divide-border rounded-lg border border-border"
        >
          {entries.map((e) => (
            <HistoryRow key={e.id} entry={e} appName={appName} />
          ))}
        </ol>
      </CardContent>
    </Card>
  )
}

function HistoryRow({
  entry,
  appName,
}: Readonly<{ entry: HistoryEntry; appName: string }>) {
  const { t } = useTranslation('deployTimeline')
  const KindIcon = KIND_ICON[entry.kind]
  const duration =
    entry.durationMs === null ? null : formatDurationMs(entry.durationMs)
  const status = t(`appHistory.status.${entry.statusKey}`)
  const kind = t(`appHistory.kind.${entry.kind}`)

  return (
    <li className="flex flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2 text-xs">
      <span className="flex min-w-24 items-center gap-1.5 font-medium text-foreground">
        <KindIcon className="size-5 shrink-0" aria-hidden="true" />
        {entry.attemptId ? (
          <Link
            to="/apps/$name/deploys/$deployId/logs"
            params={{ name: appName, deployId: entry.attemptId }}
            className="hover:underline"
          >
            {kind}
          </Link>
        ) : (
          kind
        )}
      </span>
      <StatusPill size="sm" tone={entry.tone} label={status} />
      {entry.who ? (
        <span className="inline-flex items-center gap-1 text-muted-foreground">
          <UserGlassIcon className="size-5" aria-hidden="true" />
          {entry.who}
        </span>
      ) : null}
      {entry.sha ? (
        <span className="inline-flex items-center gap-1 font-mono text-muted-foreground">
          <CommitGraphGlassIcon className="size-5" aria-hidden="true" />
          {entry.sha}
        </span>
      ) : null}
      {duration ? (
        <span className="inline-flex items-center gap-1 text-muted-foreground">
          <ClockGlassIcon className="size-5" aria-hidden="true" />
          {duration}
        </span>
      ) : null}
      <span className="ml-auto text-muted-foreground">
        <RelativeTime at={entry.at} />
      </span>
    </li>
  )
}
