import { useEffect, useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useDeployDetailT } from '../../../../../components/deployDetail/useDeployDetailT'
import { useLogStream } from '../../../../../hooks/useLogStream'
import { useDeployStepStream } from '../../../../../hooks/useDeployStepStream'
import { useNowTick } from '../../../../../hooks/useNowTick'
import {
  buildDeployLogStreamUrl,
  deployLogDownloadURL,
} from '../../../../../queries/deployLogs'
import { buildDeployStepStreamUrl } from '../../../../../queries/deploySteps'
import { deployAttemptsQueryOptions } from '../../../../../queries/deployAttempts'
import { useDeployProgress } from '../../../../../hooks/useDeployProgress'
import { useApp } from '../../../../../queries/apps'
import { useGitSource } from '../../../../../queries/gitSources'
import {
  computeDeployStages,
  computeRolloutSubStages,
} from '../../../../../lib/deployStages'
import { deriveDeployState } from '../../../../../lib/deployState'
import { summarizeAppStatus } from '../../../../../lib/appStatus'
import { Breadcrumbs } from '../../../../../components/Breadcrumbs'
import { LogTerminal } from '../../../../../components/LogTerminal'
import { DeployHeader } from '../../../../../components/DeployHeader'
import {
  DeployStageTabs,
  type StageTab,
} from '../../../../../components/DeployStageTabs'
import { DeployFailureSummaryCard } from '../../../../../components/DeployFailureSummaryCard'
import { DeployExitCard } from '../../../../../components/DeployExitCard'
import { deployStageAnchorId } from '../../../../../lib/deployFailureSummary'
import { DeployNewErrorTypes } from '../../../../../components/DeployNewErrorTypes'
import { DeployQuickLinks } from '../../../../../components/DeployQuickLinks'
import { useProbeAttempts } from '../../../../../queries/probeAttempts'
import { SupplyChainSection } from '../../../../../components/SupplyChainSection'
import { SecretScanSection } from '../../../../../components/SecretScanSection'
import { DeployStatusCard } from '../../../../../components/deployDetail/DeployStatusCard'
import { DeployLiveStatus } from '../../../../../components/deployDetail/DeployLiveStatus'
import { StreamDropBanner } from '../../../../../components/deployDetail/StreamDropBanner'
import { BuildTab } from '../../../../../components/deployDetail/BuildTab'
import { RolloutTab } from '../../../../../components/deployDetail/RolloutTab'
import { buildTimelineRows } from '../../../../../components/deployDetail/timelineRows'
import { Skeleton } from '@/components/ui/skeleton'

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

// Rollouts longer than this are flagged as stuck; env-tunable per deployment.
const STUCK_AFTER_MS =
  Number(import.meta.env.VITE_DEPLOY_STUCK_AFTER_SECONDS ?? 120) * 1000

// Keep the stream open briefly after a terminal status so the last lines land.
const CLOSE_GRACE_MS = 5000

// The deploy detail page: one derived status card (lib/deployState.ts)
// above Build, Roll out and Supply chain tabs. Route-level code split so the
// dashboard shell never ships the SSE hook or the log terminal's bundle.
export const Route = createFileRoute('/apps/$name/deploys/$deployId/logs')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    queryClient.ensureQueryData(deployAttemptsQueryOptions(name)),
  component: DeployLogsPage,
  pendingComponent: DeployLogsSkeleton,
})

function DeployLogsPage() {
  const { t, msg } = useDeployDetailT()
  const { name, deployId } = Route.useParams()
  const { attempts, conditions } = useDeployProgress(name)
  const attempt = attempts.find((a) => a.id === deployId)
  const attemptFinished =
    attempt !== undefined && TERMINAL_STATUSES.has(attempt.status)

  // Close the stream only after a short grace so the final lines still land.
  const [graceOver, setGraceOver] = useState(false)
  useEffect(() => {
    if (!attemptFinished) return
    const id = window.setTimeout(() => {
      setGraceOver(true)
    }, CLOSE_GRACE_MS)
    return () => {
      window.clearTimeout(id)
    }
  }, [attemptFinished])
  const streamClosed = attemptFinished && graceOver

  const {
    lines,
    connectionState,
    isPaused,
    pause,
    resume,
    transport,
    lastEventAt,
    retryAt,
    retry,
    switchToPolling,
  } = useLogStream(buildDeployLogStreamUrl(name, deployId), {
    closed: streamClosed,
    pollUrl: deployLogDownloadURL(name, deployId),
  })
  const { steps } = useDeployStepStream(
    buildDeployStepStreamUrl(name, deployId),
  )
  const { data: app } = useApp(name)
  const { data: gitSource } = useGitSource(name)
  const { data: probeAttempts } = useProbeAttempts(
    name,
    deployId,
    attempt?.status === 'running',
  )
  const nowMs = useNowTick(
    (attempt !== undefined && !attemptFinished) ||
      attempt?.status === 'succeeded',
  )
  const [picked, setPicked] = useState<string | null>(null)

  if (!attempt) {
    return (
      <div className="flex min-h-0 flex-1 flex-col gap-2">
        <p className="text-sm text-muted-foreground">
          {t('page.notInHistory')}
        </p>
        <LogTerminal
          lines={lines}
          isPaused={isPaused}
          pause={pause}
          resume={resume}
          heightClassName="h-[50vh]"
        />
      </div>
    )
  }

  const noBuildStep =
    attempt.source !== undefined && IMAGE_ONLY_SOURCES.has(attempt.source)
  const isLatestAttempt = attempts[0]?.id === deployId
  const stages = computeDeployStages(attempt, conditions, isLatestAttempt)
  const subStages = computeRolloutSubStages(
    attempt,
    conditions,
    isLatestAttempt,
  )
  const isStatic =
    gitSource?.build_type === 'static' ||
    /static/i.test(attempt.detected_framework ?? '')
  const state = deriveDeployState({
    attempt,
    stages,
    conditions,
    isLatestAttempt,
    isStatic,
    appStatusLabel: summarizeAppStatus(conditions).label,
    nowMs,
    stuckAfterMs: STUCK_AFTER_MS,
  })

  const failedStage = stages.find((s) => s.status === 'failed')
  const tab = picked ?? (failedStage?.key === 'rollout' ? 'rollout' : 'build')
  const rolloutTabStage =
    isStatic && state.phase === 'live'
      ? { ...stages[1], status: 'done' as const }
      : stages[1]
  const tabs: StageTab[] = [
    {
      id: 'build',
      label: noBuildStep ? t('tabs.image') : t('tabs.build'),
      stage: stages[0],
      hint: lines.length > 0 ? lines.length.toLocaleString() : undefined,
    },
    { id: 'rollout', label: t('tabs.rollout'), stage: rolloutTabStage },
    { id: 'supply', label: t('tabs.supply') },
  ]
  const rows = buildTimelineRows(
    stages[0],
    subStages,
    state,
    attempt.finished_at,
  )

  return (
    <div className="flex h-full flex-col gap-5 overflow-y-auto">
      <Breadcrumbs
        projectId={app.project_id}
        environmentId={app.environment_id}
        appName={name}
        page="Deploy"
      />

      <DeployHeader
        appName={name}
        attempt={attempt}
        domain={app.domains?.[0]}
        branch={attempt.source === 'webhook' ? gitSource?.branch : undefined}
        isLive={attempt.rollout_state === 'serving'}
        displayStatus={{
          tone: state.tone,
          label: msg(state.label),
          live: state.phase === 'building' || state.phase === 'rolling_out',
        }}
      />
      <DeployStatusCard
        appName={name}
        state={state}
        onOpenBuild={() => {
          setPicked('build')
        }}
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
      <DeployExitCard
        appName={name}
        deployId={deployId}
        failed={attempt.status === 'failed'}
      />

      <div className="flex min-h-0 flex-1 flex-col gap-3">
        <div className="flex flex-wrap items-end justify-between gap-2">
          <DeployStageTabs tabs={tabs} value={tab} onChange={setPicked} />
          <div className="pb-1">
            <DeployLiveStatus
              transport={transport}
              connectionState={connectionState}
              lastEventAt={lastEventAt}
              retryAt={retryAt}
              onRetry={retry}
            />
          </div>
        </div>

        <StreamDropBanner
          transport={transport}
          connectionState={connectionState}
          retryAt={retryAt}
          onRetry={retry}
          onSwitchToPolling={switchToPolling}
          downloadUrl={deployLogDownloadURL(name, deployId)}
        />

        {tab === 'build' ? (
          <div
            id={deployStageAnchorId('build')}
            tabIndex={-1}
            className="outline-none"
          >
            <BuildTab
              appName={name}
              deployId={deployId}
              attempt={attempt}
              gitSource={gitSource}
              lines={lines}
              steps={steps}
              noBuildStep={noBuildStep}
              finished={attemptFinished}
              isPaused={isPaused}
              pause={pause}
              resume={resume}
            />
          </div>
        ) : null}

        {tab === 'rollout' ? (
          <div
            id={deployStageAnchorId('rollout')}
            tabIndex={-1}
            className="outline-none"
          >
            <RolloutTab
              appName={name}
              deployId={deployId}
              rows={rows}
              state={state}
              nowMs={nowMs}
              conditions={conditions}
              probeAttempts={probeAttempts}
            />
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
      <div className="space-y-1.5">
        <Skeleton className="h-7 w-48" />
        <Skeleton className="h-3 w-64 max-w-full" />
      </div>
      <Skeleton className="h-28 w-full rounded-lg" />
      <Skeleton className="h-9 w-72 max-w-full" />
      <Skeleton className="h-64 w-full rounded-lg" />
    </div>
  )
}
