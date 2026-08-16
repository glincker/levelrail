import { createFileRoute } from '@tanstack/react-router'
import { TerminalIcon } from '@phosphor-icons/react/dist/ssr'
import { useLogStream } from '../../../../../hooks/useLogStream'
import { buildDeployLogStreamUrl } from '../../../../../queries/deployLogs'
import {
  deployAttemptsQueryOptions,
  useDeployAttempts,
} from '../../../../../queries/deployAttempts'
import { LogConnectionBadge } from '../../../../../components/LogConnectionBadge'
import { LogTerminal } from '../../../../../components/LogTerminal'
import { BuildLogHints } from '../../../../../components/BuildLogHints'

// 'image' attempts (a bare image-tag redeploy/rollback) have no build
// step, so they never produce log lines: see types/deployAttempt.ts's
// own doc comment. Without this, the empty state read as a permanent,
// pulsing "Waiting for log output..." with nothing ever arriving to
// explain why.
const NO_BUILD_STEP_MESSAGE =
  'This deployment used a pre-built Docker image, so there is no build output.'

// Live Build Log view, frontend-plan.md section 1: the literal target of
// the rule that the dashboard should not ship the log viewer's bundle.
// This route and routes/apps/$name/logs.tsx's live tab are the only two
// places in the app that import the SSE hook, the ANSI stripper, and
// TanStack Virtual tuned for monospace log rows: both are route-level
// chunks, neither of which ships in the shared/main bundle. Verified via
// `npm run build`'s per-chunk output.
//
// The log stream itself is a live, append-only feed from useLogStream,
// deliberately a separate data layer from Query (frontend-plan.md section
// 2: a live stream does not fit Query's freshness/staleness model). The
// loader below only primes the deploy-attempts list already fetched by
// deploys/index.tsx, so this route can tell an 'image' attempt (no build
// step, see NO_BUILD_STEP_MESSAGE) from one still genuinely awaiting
// output, without a second network round-trip in the common case of
// navigating here from that list.
export const Route = createFileRoute('/apps/$name/deploys/$deployId/logs')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    queryClient.ensureQueryData(deployAttemptsQueryOptions(name)),
  component: DeployLogsPage,
})

function DeployLogsPage() {
  const { name, deployId } = Route.useParams()
  const url = buildDeployLogStreamUrl(name, deployId)
  const { lines, connectionState, isPaused, pause, resume } = useLogStream(url)
  const { data: attempts } = useDeployAttempts(name)
  const attempt = attempts.find((a) => a.id === deployId)
  const noBuildStep = attempt?.source === 'image'

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
        emptyStateMessage={noBuildStep ? NO_BUILD_STEP_MESSAGE : undefined}
        emptyStatePulse={!noBuildStep}
      />
    </div>
  )
}
