import { useLogStream } from '../hooks/useLogStream'
import { buildLiveLogStreamUrl } from '../queries/liveLogs'
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

export function LiveLogViewer({ appName }: { appName: string }) {
  const url = buildLiveLogStreamUrl(appName)
  const { lines, connectionState, isPaused, pause, resume } = useLogStream(url)

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
