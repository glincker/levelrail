import { createFileRoute } from '@tanstack/react-router'
import { TerminalIcon } from '@phosphor-icons/react/dist/ssr'
import { useLogStream } from '../../../../../hooks/useLogStream'
import { buildDeployLogStreamUrl } from '../../../../../queries/deployLogs'
import { LogConnectionBadge } from '../../../../../components/LogConnectionBadge'
import { LogTerminal } from '../../../../../components/LogTerminal'
import { BuildLogHints } from '../../../../../components/BuildLogHints'

// Live Build Log view, frontend-plan.md section 1: the literal target of
// the rule that the dashboard should not ship the log viewer's bundle.
// This route and routes/apps/$name/logs.tsx's live tab are the only two
// places in the app that import the SSE hook, the ANSI stripper, and
// TanStack Virtual tuned for monospace log rows: both are route-level
// chunks, neither of which ships in the shared/main bundle. Verified via
// `npm run build`'s per-chunk output.
//
// No loader here (contrast web/src/routes/apps/index.tsx): this route has
// no fetched resource to prime into TanStack Query's cache. The log
// stream is a live, append-only feed from useLogStream, which is
// deliberately a separate data layer from Query (frontend-plan.md section
// 2: a live stream does not fit Query's freshness/staleness model).
export const Route = createFileRoute('/apps/$name/deploys/$deployId/logs')({
  component: DeployLogsPage,
})

function DeployLogsPage() {
  const { name, deployId } = Route.useParams()
  const url = buildDeployLogStreamUrl(name, deployId)
  const { lines, connectionState, isPaused, pause, resume } = useLogStream(url)

  return (
    <div className="flex h-full flex-col gap-3">
      <header className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <h1 className="flex items-center gap-1.5 text-lg font-semibold text-foreground">
            <TerminalIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            Build logs
          </h1>
          <p className="truncate text-xs text-muted-foreground">
            {name} / deploy {deployId}
            {lines.length > 0
              ? ` · ${lines.length.toLocaleString()} lines`
              : ''}
          </p>
        </div>
        <LogConnectionBadge state={connectionState} />
      </header>

      <BuildLogHints lines={lines} />

      <LogTerminal
        lines={lines}
        isPaused={isPaused}
        pause={pause}
        resume={resume}
        heightClassName="h-[70vh]"
      />
    </div>
  )
}
