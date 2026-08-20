import { WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Link } from '@tanstack/react-router'
import { useLogStream } from '../hooks/useLogStream'
import { buildLiveLogStreamUrl } from '../queries/liveLogs'
import type { ReconcileCondition } from '../types/deploy'
import { LogConnectionBadge } from './LogConnectionBadge'
import { LogTerminal } from './LogTerminal'

// Live-tailing terminal view for one app's running container output:
// GET /api/v1/apps/{name}/logs/stream (internal/api/live_logs.go), which
// backfills a short window of recent context before switching to a live
// tail (see that handler's own doc comment for the backfill-to-live
// handoff). Structurally this is the app-log sibling of
// routes/apps/$name/deploys/$deployId/logs.tsx's build-log viewer: same
// hook (useLogStream), same virtualized monospace terminal look, same
// auto-scroll/pause behavior, because it's the same kind of problem
// (tail an append-only, possibly high-volume text stream without
// yanking the reader's scroll position). Kept as a separate component
// rather than folded into that route file: that route owns its own
// full-page header and has no search/live toggle to coordinate with,
// this one is embedded inside routes/apps/$name/logs.tsx alongside
// LogSearchPanel.
//
// The terminal body itself (scroll-follow, virtualized rows, resume
// button) is LogTerminal, shared with the deploy-logs route: see that
// component's own doc comment for why the two views share the body but
// not the useLogStream call or the status row above it.

export function LiveLogViewer({
  appName,
  conditions,
}: {
  appName: string
  /** The app's current reconcile conditions (useDeployStatus), so an
   *  empty tail can explain why instead of leaving a bare "waiting"
   *  message: a container that never ran at all looks identical, from
   *  this stream's point of view, to a healthy one that just hasn't
   *  logged anything yet. Optional: a caller with no conditions handy
   *  still gets the old plain message, never a crash. */
  conditions?: ReconcileCondition[]
}) {
  const url = buildLiveLogStreamUrl(appName)
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
              to="/apps/$name/deploys"
              params={{ name: appName }}
              className="mt-1.5 inline-block underline underline-offset-2"
            >
              View deploy history to troubleshoot
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
