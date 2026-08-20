import { createFileRoute } from '@tanstack/react-router'
import { TerminalIcon } from '@phosphor-icons/react/dist/ssr'
import { useLogStream } from '../../../../../hooks/useLogStream'
import { buildDeployLogStreamUrl } from '../../../../../queries/deployLogs'
import {
  deployAttemptsQueryOptions,
  useDeployAttempts,
} from '../../../../../queries/deployAttempts'
import { useDeployStatus } from '../../../../../queries/deploys'
import { computeDeployStages } from '../../../../../lib/deployStages'
import { LogConnectionBadge } from '../../../../../components/LogConnectionBadge'
import { LogTerminal } from '../../../../../components/LogTerminal'
import { BuildLogHints } from '../../../../../components/BuildLogHints'
import { DeployStageTimeline } from '../../../../../components/DeployStageTimeline'

// 'image' attempts (a bare image-tag redeploy/rollback) have no build
// step, so they never produce log lines: see types/deployAttempt.ts's
// own doc comment. Without this, the empty state read as a permanent,
// pulsing "Waiting for log output..." with nothing ever arriving to
// explain why.
const NO_BUILD_STEP_MESSAGE =
  'This deployment used a pre-built Docker image, so there is no build output.'

// Live deploy view: a stage timeline (DeployStageTimeline, the primary
// at-a-glance element) above the live build/deploy output. Route-level
// code split, same as routes/apps/$name/logs.tsx's live tab, so the
// dashboard shell never ships the SSE hook or the log terminal's bundle.
//
// The log stream is a live feed from useLogStream, deliberately separate
// from Query's cache model. Conditions come from the parent route's
// already-primed useDeployStatus cache, no extra fetch; the loader below
// only primes deploy-attempts, reused from deploys/index.tsx.
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
  const { data: conditions } = useDeployStatus(name)
  const attempt = attempts.find((a) => a.id === deployId)
  const noBuildStep = attempt?.source === 'image'
  const isLatestAttempt = attempts[0]?.id === deployId

  return (
    <div className="flex h-full flex-col gap-4">
      <header className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <h1 className="flex items-center gap-1.5 text-lg font-semibold text-foreground">
            <TerminalIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            Deploy
          </h1>
          <p className="truncate text-xs text-muted-foreground">
            {name} / deploy {deployId}
          </p>
        </div>
        <LogConnectionBadge state={connectionState} />
      </header>

      {attempt ? (
        <DeployStageTimeline
          stages={computeDeployStages(attempt, conditions, isLatestAttempt)}
        />
      ) : null}

      <div className="min-h-0 flex-1 flex flex-col gap-2">
        <div className="flex items-center justify-between gap-3">
          <h2 className="text-sm font-medium text-muted-foreground uppercase tracking-wide">
            Live output
          </h2>
          <p className="text-xs text-muted-foreground">
            {lines.length > 0
              ? `${lines.length.toLocaleString()} lines`
              : ''}
          </p>
        </div>

        <BuildLogHints lines={lines} />

        <LogTerminal
          lines={lines}
          isPaused={isPaused}
          pause={pause}
          resume={resume}
          heightClassName="h-[50vh]"
          emptyStateMessage={noBuildStep ? NO_BUILD_STEP_MESSAGE : undefined}
          emptyStatePulse={!noBuildStep}
        />
      </div>
    </div>
  )
}
