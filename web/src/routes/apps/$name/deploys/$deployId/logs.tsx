import { createFileRoute } from '@tanstack/react-router'
import { TerminalIcon } from '@phosphor-icons/react/dist/ssr'
import { useLogStream, type LogLine } from '../../../../../hooks/useLogStream'
import { buildDeployLogStreamUrl } from '../../../../../queries/deployLogs'
import {
  deployAttemptsQueryOptions,
  useDeployAttempts,
} from '../../../../../queries/deployAttempts'
import { useDeployStatus } from '../../../../../queries/deploys'
import { useApp } from '../../../../../queries/apps'
import { useGitSource } from '../../../../../queries/gitSources'
import {
  computeDeployStages,
  type DeployStage,
} from '../../../../../lib/deployStages'
import type { ReconcileCondition } from '../../../../../types/deploy'
import { stripAnsiCodes } from '../../../../../lib/ansi'
import { Breadcrumbs } from '../../../../../components/Breadcrumbs'
import { LogConnectionBadge } from '../../../../../components/LogConnectionBadge'
import { LogTerminal } from '../../../../../components/LogTerminal'
import { BuildLogHints } from '../../../../../components/BuildLogHints'
import { DeploySection } from '../../../../../components/DeploySection'
import { DeployMetaCard } from '../../../../../components/DeployMetaCard'
import { DeployQuickLinks } from '../../../../../components/DeployQuickLinks'
import { ConditionsPanel } from '../../../../../components/ConditionsPanel'

// 'image' attempts (a bare image-tag redeploy/rollback) have no build
// step, so they never produce log lines: see types/deployAttempt.ts's
// own doc comment. Without this, the empty state read as a permanent,
// pulsing "Waiting for log output..." with nothing ever arriving to
// explain why.
const NO_BUILD_STEP_MESSAGE =
  'This deployment used a pre-built Docker image, so there is no build output.'

// The deploy detail page: a real metadata summary (DeployMetaCard) above
// the two real pipeline stages (DeploySection, one per lib/deployStages.ts
// entry) each collapsed to a status and a one-line summary by default,
// expanding into their own real content (the live/persisted build log for
// Build, the actual reconcile conditions for Roll out). Route-level code
// split, same as routes/apps/$name/logs.tsx's live tab, so the dashboard
// shell never ships the SSE hook or the log terminal's bundle.
//
// The log stream is a live feed from useLogStream, deliberately separate
// from Query's cache model. Conditions and the app resource come from the
// parent route's already-primed caches, no extra fetch; the loader below
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
  const { data: app } = useApp(name)
  // Non-suspense: most apps have no git source connected, the common
  // steady state, not an exceptional one (see queries/gitSources.ts).
  const { data: gitSource } = useGitSource(name)
  const attempt = attempts.find((a) => a.id === deployId)
  const noBuildStep = attempt?.source === 'image'
  const isLatestAttempt = attempts[0]?.id === deployId
  const stages = attempt
    ? computeDeployStages(attempt, conditions, isLatestAttempt)
    : null

  return (
    <div className="flex h-full flex-col gap-4 overflow-y-auto">
      <Breadcrumbs
        projectId={app.project_id}
        environmentId={app.environment_id}
        appName={name}
        page="Deploy logs"
      />
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

      {attempt && stages ? (
        <>
          <DeployMetaCard
            attempt={attempt}
            stages={stages}
            domains={app.domains}
            branch={
              attempt.source === 'webhook' ? gitSource?.branch : undefined
            }
          />

          <div className="space-y-3">
            <DeploySection
              stage={stages[0]}
              summary={buildStageSummary(stages[0], lines)}
              defaultOpen={
                stages[0].status === 'running' || stages[0].status === 'failed'
              }
            >
              <div className="space-y-2">
                <div className="flex items-center justify-between gap-3">
                  <h3 className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
                    Build output
                  </h3>
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
                  heightClassName="h-[40vh]"
                  emptyStateMessage={
                    noBuildStep ? NO_BUILD_STEP_MESSAGE : undefined
                  }
                  emptyStatePulse={!noBuildStep}
                />
              </div>
            </DeploySection>

            <DeploySection
              stage={stages[1]}
              summary={rolloutStageSummary(stages[1], conditions)}
              defaultOpen={stages[1].status === 'failed'}
            >
              <ConditionsPanel conditions={conditions} />
            </DeploySection>
          </div>

          <DeployQuickLinks appName={name} />
        </>
      ) : (
        <div className="min-h-0 flex-1 flex flex-col gap-2">
          <p className="text-sm text-muted-foreground">
            This deploy attempt is not in this app&rsquo;s current history.
          </p>
          <LogTerminal
            lines={lines}
            isPaused={isPaused}
            pause={pause}
            resume={resume}
            heightClassName="h-[50vh]"
          />
        </div>
      )}
    </div>
  )
}

function buildStageSummary(stage: DeployStage, lines: LogLine[]): string {
  if (stage.status === 'skipped') {
    return stage.detail ?? 'No build step ran.'
  }
  if (stage.status === 'failed') {
    return stage.detail || 'Build failed.'
  }
  if (stage.status === 'done') {
    return 'Build finished.'
  }
  if (stage.status === 'running') {
    const last = lines[lines.length - 1]
    return last ? stripAnsiCodes(last.line) : 'Waiting for build output...'
  }
  // pending/unknown never occur for the build stage in practice (see
  // computeDeployStages), kept only so this stays exhaustive.
  return 'Not started yet.'
}

function rolloutStageSummary(
  stage: DeployStage,
  conditions: ReconcileCondition[],
): string {
  if (stage.status === 'skipped' || stage.status === 'unknown') {
    return stage.detail ?? 'Not tracked.'
  }
  if (stage.status === 'pending') {
    return 'Waiting for the build to finish.'
  }
  if (stage.status === 'failed') {
    return stage.detail || 'Roll out failed.'
  }
  if (stage.status === 'done') {
    const deployed = conditions.find((c) => c.Reason === 'Deployed')
    return deployed?.Message || 'Deployed successfully.'
  }
  const latest = [...conditions].sort(
    (a, b) =>
      new Date(b.LastTransitionTime).getTime() -
      new Date(a.LastTransitionTime).getTime(),
  )[0]
  return latest?.Message || 'Rolling out the new containers.'
}
