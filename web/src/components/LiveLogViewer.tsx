import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useLogStream } from '../hooks/useLogStream'
import { buildLiveLogStreamUrl } from '../queries/liveLogs'
import type { ReconcileCondition } from '../types/deploy'
import { ContainerExitBanner } from './ContainerExitBanner'
import { LogConnectionBadge } from './LogConnectionBadge'
import { LogTerminal } from './LogTerminal'

// Live-tailing terminal for one app's container output (GET
// /api/v1/apps/{name}/logs/stream). The stream's status event says whether
// the container runs; when it does not, the lines are its last output and
// the banner says how it stopped instead of a green "Live".

type LineCountKey =
  'liveLogs.lineCount' | 'liveLogs.lastOutput' | 'liveLogs.waiting'

export function LiveLogViewer({
  appName,
  conditions,
}: {
  appName: string
  /** The app's reconcile conditions, so an empty tail with no status can
   *  still explain why nothing is running. */
  conditions?: ReconcileCondition[]
}) {
  const { t } = useTranslation('observability')
  const url = buildLiveLogStreamUrl(appName)
  const { lines, connectionState, isPaused, pause, resume, runStatus } =
    useLogStream(url)
  const stopped = runStatus !== null && !runStatus.running
  const problem =
    !stopped && lines.length === 0
      ? conditions?.find((c) => c.Status === 'False')
      : undefined

  let countKey: LineCountKey = 'liveLogs.waiting'
  if (lines.length > 0) {
    countKey = stopped ? 'liveLogs.lastOutput' : 'liveLogs.lineCount'
  }

  return (
    <div className="flex h-full flex-col gap-2">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">
          {t(countKey, { count: lines.length })}
        </p>
        <LogConnectionBadge state={connectionState} notRunning={stopped} />
      </div>

      {runStatus && !runStatus.running ? (
        <ContainerExitBanner
          exitCode={runStatus.exit_code}
          oomKilled={runStatus.oom_killed}
          finishedAt={runStatus.finished_at}
          message={runStatus.message}
          reason={runStatus.reason}
          showHint
        />
      ) : null}

      {problem ? (
        <div className="flex items-start gap-2.5 rounded-lg border border-tone-warning-border bg-tone-warning-soft p-3 text-tone-warning">
          <TriangleAlertGlassIcon
            className="mt-0.5 size-6 shrink-0 text-tone-warning"
            aria-hidden="true"
          />
          <div className="min-w-0 flex-1 text-sm">
            <p className="font-medium">
              {t('liveLogs.noContainer', { reason: problem.Reason })}
            </p>
            {problem.Message ? (
              <p className="mt-0.5 text-foreground">{problem.Message}</p>
            ) : null}
            <Link
              to="/apps/$name/deploys"
              params={{ name: appName }}
              className="mt-1.5 inline-block underline underline-offset-2"
            >
              {t('liveLogs.viewDeploys')}
            </Link>
          </div>
        </div>
      ) : null}

      <LogTerminal
        lines={lines}
        isPaused={isPaused}
        pause={pause}
        resume={resume}
        heightClassName="h-[65vh]"
      />
    </div>
  )
}
