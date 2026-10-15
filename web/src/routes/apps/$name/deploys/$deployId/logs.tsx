import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { DownloadSimpleIcon } from '@phosphor-icons/react/dist/ssr'
import { useLogStream } from '../../../../../hooks/useLogStream'
import { useDeployStepStream } from '../../../../../hooks/useDeployStepStream'
import {
  buildDeployLogStreamUrl,
  deployLogDownloadURL,
} from '../../../../../queries/deployLogs'
import { buildDeployStepStreamUrl } from '../../../../../queries/deploySteps'
import { DeployStepFeed } from '../../../../../components/DeployStepFeed'
import { buttonVariants } from '../../../../../components/ui/button'
import { deployAttemptsQueryOptions } from '../../../../../queries/deployAttempts'
import { useDeployProgress } from '../../../../../hooks/useDeployProgress'
import { useApp } from '../../../../../queries/apps'
import { useGitSource } from '../../../../../queries/gitSources'
import {
  computeDeployStages,
  computeRolloutSubStages,
  type DeployStage,
} from '../../../../../lib/deployStages'
import type { ReconcileCondition } from '../../../../../types/deploy'
import { Breadcrumbs } from '../../../../../components/Breadcrumbs'
import { LogConnectionBadge } from '../../../../../components/LogConnectionBadge'
import { LogTerminal } from '../../../../../components/LogTerminal'
import { BuildLogHints } from '../../../../../components/BuildLogHints'
import { DeployHeader } from '../../../../../components/DeployHeader'
import {
  DeployStageTabs,
  type StageTab,
} from '../../../../../components/DeployStageTabs'
import { DeployFailureSummaryCard } from '../../../../../components/DeployFailureSummaryCard'
import { deployStageAnchorId } from '../../../../../lib/deployFailureSummary'
import { DeployNewErrorTypes } from '../../../../../components/DeployNewErrorTypes'
import { DeployQuickLinks } from '../../../../../components/DeployQuickLinks'
import { DeployStageTimeline } from '../../../../../components/DeployStageTimeline'
import { ConditionsPanel } from '../../../../../components/ConditionsPanel'
import { DeployGatePanel } from '../../../../../components/DeployGatePanel'
import { ProbeAttemptsPanel } from '../../../../../components/ProbeAttemptsPanel'
import { useProbeAttempts } from '../../../../../queries/probeAttempts'
import { SupplyChainSection } from '../../../../../components/SupplyChainSection'
import { SecretScanSection } from '../../../../../components/SecretScanSection'
import { Skeleton } from '@/components/ui/skeleton'

// 'image' attempts (a bare image-tag redeploy/rollback) have no build
// step, so they never produce log lines: see types/deployAttempt.ts's
// own doc comment. Without this, the empty state read as a permanent,
// pulsing "Waiting for log output..." with nothing ever arriving to
// explain why.
const NO_BUILD_STEP_MESSAGE =
  'This deployment used a pre-built Docker image, so there is no build output.'

// A finished deploy that produced no build lines will never produce any, so
// say that instead of implying output is still on its way.
const NO_OUTPUT_RECORDED_MESSAGE =
  'No build output was recorded for this deployment.'

// Sources whose deploy pulls an image as is: no build runs.
const IMAGE_ONLY_SOURCES: ReadonlySet<string> = new Set([
  'image',
  'compose',
  'promote',
  'clone',
  'auto_rollback',
])

const TERMINAL_STATUSES: ReadonlySet<string> = new Set([
  'succeeded',
  'failed',
  'superseded',
  'canceled',
])

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
  pendingComponent: DeployLogsSkeleton,
})

function DeployLogsPage() {
  const { name, deployId } = Route.useParams()
  const url = buildDeployLogStreamUrl(name, deployId)
  const { lines, connectionState, isPaused, pause, resume } = useLogStream(url)
  const { steps } = useDeployStepStream(
    buildDeployStepStreamUrl(name, deployId),
  )
  const { attempts, conditions } = useDeployProgress(name)
  const { data: app } = useApp(name)
  // Non-suspense: most apps have no git source connected, the common
  // steady state, not an exceptional one (see queries/gitSources.ts).
  const { data: gitSource } = useGitSource(name)
  const attempt = attempts.find((a) => a.id === deployId)
  const { data: probeAttempts } = useProbeAttempts(
    name,
    deployId,
    attempt?.status === 'running',
  )
  const noBuildStep =
    attempt?.source !== undefined && IMAGE_ONLY_SOURCES.has(attempt.source)
  const attemptFinished =
    attempt !== undefined && TERMINAL_STATUSES.has(attempt.status)
  const noOutputWillCome = noBuildStep || attemptFinished
  const isLatestAttempt = attempts[0]?.id === deployId
  const stages = attempt
    ? computeDeployStages(attempt, conditions, isLatestAttempt)
    : null
  const rolloutSubStages = attempt
    ? computeRolloutSubStages(attempt, conditions, isLatestAttempt)
    : null

  const failedStage = stages?.find((s) => s.status === 'failed')
  const [picked, setPicked] = useState<string | null>(null)
  const tab = picked ?? (failedStage?.key === 'rollout' ? 'rollout' : 'build')
  const tabs: StageTab[] = stages
    ? [
        {
          id: 'build',
          label: noBuildStep ? 'Image' : 'Build',
          stage: stages[0],
          hint: lines.length > 0 ? lines.length.toLocaleString() : undefined,
        },
        { id: 'rollout', label: 'Roll out', stage: stages[1] },
        { id: 'supply', label: 'Supply chain' },
      ]
    : []

  return (
    <div className="flex h-full flex-col gap-5 overflow-y-auto">
      <Breadcrumbs
        projectId={app.project_id}
        environmentId={app.environment_id}
        appName={name}
        page="Deploy"
      />

      {attempt && stages ? (
        <>
          <DeployHeader
            appName={name}
            attempt={attempt}
            domain={app.domains?.[0]}
            branch={
              attempt.source === 'webhook' ? gitSource?.branch : undefined
            }
            isLive={attempt.rollout_state === 'serving'}
          />
          <DeployNewErrorTypes appName={name} startedAt={attempt.started_at} />

          <DeployFailureSummaryCard
            appName={name}
            attempt={attempt}
            attempts={attempts}
            stages={stages}
            conditions={conditions}
            lines={lines}
          />

          <div className="flex min-h-0 flex-1 flex-col gap-3">
            <div className="flex flex-wrap items-end justify-between gap-2">
              <DeployStageTabs tabs={tabs} value={tab} onChange={setPicked} />
              <div className="flex items-center gap-3 pb-1">
                <LogConnectionBadge state={connectionState} />
                {tab === 'build' && !noBuildStep ? (
                  <a
                    href={deployLogDownloadURL(name, deployId)}
                    download
                    className={buttonVariants({
                      variant: 'outline',
                      size: 'sm',
                    })}
                  >
                    <DownloadSimpleIcon
                      className="size-3.5"
                      aria-hidden="true"
                    />
                    Download
                  </a>
                ) : null}
              </div>
            </div>

            {tab === 'build' ? (
              <div
                id={deployStageAnchorId('build')}
                tabIndex={-1}
                className="space-y-3 outline-none"
              >
                {!noBuildStep && steps.length > 0 ? (
                  <details className="rounded-lg border border-border px-3 py-2">
                    <summary className="cursor-pointer text-sm font-medium text-foreground">
                      Build steps ({steps.length})
                    </summary>
                    <div className="pt-2">
                      <DeployStepFeed steps={steps} />
                    </div>
                  </details>
                ) : null}
                <BuildLogHints lines={lines} />
                <LogTerminal
                  lines={lines}
                  isPaused={isPaused}
                  pause={pause}
                  resume={resume}
                  heightClassName="h-[calc(100dvh-24rem)] min-h-80"
                  emptyStateMessage={
                    noBuildStep
                      ? NO_BUILD_STEP_MESSAGE
                      : attemptFinished
                        ? NO_OUTPUT_RECORDED_MESSAGE
                        : undefined
                  }
                  emptyStatePulse={!noOutputWillCome}
                  isFinished={attempt.status !== 'running'}
                />
              </div>
            ) : null}

            {tab === 'rollout' ? (
              <div
                id={deployStageAnchorId('rollout')}
                tabIndex={-1}
                className="space-y-4 outline-none"
              >
                <p className="text-sm text-muted-foreground">
                  {rolloutStageSummary(stages[1], conditions)}
                </p>
                {rolloutSubStages ? (
                  <DeployStageTimeline stages={rolloutSubStages} />
                ) : null}
                <DeployGatePanel appName={name} deployId={deployId} />
                <ConditionsPanel conditions={conditions} />
                {probeAttempts && probeAttempts.length > 0 ? (
                  <ProbeAttemptsPanel attempts={probeAttempts} />
                ) : null}
              </div>
            ) : null}

            {tab === 'supply' ? (
              <div className="space-y-6">
                <SupplyChainSection
                  appName={name}
                  deploymentId={deployId}
                  sbomPackages={attempt.sbom_packages}
                />
                <SecretScanSection appName={name} deploymentId={deployId} />
              </div>
            ) : null}
          </div>

          <DeployQuickLinks appName={name} />
        </>
      ) : (
        <div className="flex min-h-0 flex-1 flex-col gap-2">
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

// The live log stream itself isn't worth faking line by line, so this
// mirrors just the outer chrome: header, meta card, two collapsed
// pipeline sections, a full-width terminal block.
function DeployLogsSkeleton() {
  return (
    <div className="flex h-full flex-col gap-4" aria-hidden="true">
      <Skeleton className="h-4 w-56" />
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="space-y-1.5">
          <Skeleton className="h-6 w-24" />
          <Skeleton className="h-3 w-40" />
        </div>
        <Skeleton className="h-5 w-20 rounded-full" />
      </div>

      <div className="space-y-2 rounded-lg border border-border p-4">
        <Skeleton className="h-4 w-32" />
        <Skeleton className="h-4 w-full" />
        <Skeleton className="h-4 w-2/3" />
      </div>

      <div className="space-y-3">
        <Skeleton className="h-12 w-full rounded-lg" />
        <Skeleton className="h-12 w-full rounded-lg" />
      </div>

      <Skeleton className="h-[40vh] w-full rounded-lg" />
    </div>
  )
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
    const deployed = conditions.find(
      (c) => c.Reason === 'Deployed' || c.Reason === 'AlreadyRunning',
    )
    return deployed?.Message || 'Deployed successfully.'
  }
  const latest = [...conditions].sort(
    (a, b) =>
      new Date(b.LastTransitionTime).getTime() -
      new Date(a.LastTransitionTime).getTime(),
  )[0]
  return latest?.Message || 'Rolling out the new containers.'
}
