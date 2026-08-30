import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Link } from '@tanstack/react-router'
import { useLogStream } from '../hooks/useLogStream'
import { buildLiveDatabaseLogStreamUrl } from '../queries/liveDatabaseLogs'
import type { ReconcileCondition } from '../types/deploy'
import { LogConnectionBadge } from './LogConnectionBadge'
import { LogTerminal } from './LogTerminal'

// Live-tailing terminal view for one managed database's container
// output: GET /api/v1/databases/{name}/logs/stream
// (internal/api/database_logs.go), the database-kind counterpart to
// LiveLogViewer.tsx. Same backfill-then-live handoff, same terminal body
// (LogTerminal, shared rather than re-implemented). Differs only in
// where the "no container running" hint links to: apps have a deploy
// history to troubleshoot against, a database has no deploys at all, so
// this points at the database's own Overview section (engine/version/
// node summary plus ConditionsPanel) instead.
export function LiveDatabaseLogViewer({
  databaseName,
  conditions,
}: {
  databaseName: string
  /** The database's current reconcile conditions (useDatabaseStatus), same
   *  "explain why an empty tail is empty" purpose LiveLogViewer's own
   *  conditions prop serves. */
  conditions?: ReconcileCondition[]
}) {
  const url = buildLiveDatabaseLogStreamUrl(databaseName)
  const { lines, connectionState, isPaused, pause, resume } = useLogStream(url)
  const problem = lines.length === 0 ? conditions?.find((c) => c.Status === 'False') : undefined

  return (
    <div className="flex h-full flex-col gap-2">
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs text-muted-foreground">
          {lines.length > 0
            ? `${lines.length.toLocaleString()} lines (recent context, then live)`
            : 'Waiting for log output...'}
        </p>
        <LogConnectionBadge state={connectionState} />
      </div>

      {problem ? (
        <div className="flex items-start gap-2.5 rounded-lg border border-amber-200 bg-amber-50 p-3 text-amber-900 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
          <WarningIcon className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
          <div className="min-w-0 flex-1 text-sm">
            <p className="font-medium">
              No container is running to produce logs: {problem.Reason}
            </p>
            {problem.Message ? (
              <p className="mt-0.5 text-amber-800 dark:text-amber-300/90">
                {problem.Message}
              </p>
            ) : null}
            <Link
              to="/databases/$name/overview"
              params={{ name: databaseName }}
              className="mt-1.5 inline-block underline underline-offset-2"
            >
              View status to troubleshoot
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
