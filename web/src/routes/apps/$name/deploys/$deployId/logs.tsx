import { lazy, Suspense, useEffect, useState } from 'react'
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
import { DeployFailureSummaryCard } from '../../../../../components/DeployFailureSummaryCard'
import { DeployExitCard } from '../../../../../components/DeployExitCard'
import { deployStageAnchorId } from '../../../../../lib/deployFailureSummary'
import { DeployNewErrorTypes } from '../../../../../components/DeployNewErrorTypes'
import { DeployQuickLinks } from '../../../../../components/DeployQuickLinks'
import { useProbeAttempts } from '../../../../../queries/probeAttempts'
import { SupplyChainSection } from '../../../../../components/SupplyChainSection'
import { SecretScanSection } from '../../../../../components/SecretScanSection'
import { DeployExplainPanel } from '../../../../../components/deployDetail/DeployExplainPanel'
import { DeployStatusCard } from '../../../../../components/deployDetail/DeployStatusCard'
import { DeployLiveStatus } from '../../../../../components/deployDetail/DeployLiveStatus'
import { StreamDropBanner } from '../../../../../components/deployDetail/StreamDropBanner'
import { BuildTab } from '../../../../../components/deployDetail/BuildTab'
import { DeployPhases } from '../../../../../components/deployDetail/DeployPhases'
import { RolloutTab } from '../../../../../components/deployDetail/RolloutTab'
import { buildTimelineRows } from '../../../../../components/deployDetail/timelineRows'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { Link } from '@tanstack/react-router'
import { RunSection } from '../../../../../components/buildStages/RunSection'
import { RunSectionNav } from '../../../../../components/buildStages/RunSectionNav'
import { StageOverview } from '../../../../../components/buildStages/StageOverview'
import { StageLogPanel } from '../../../../../components/buildStages/StageLogPanel'
import { StageWaterfall } from '../../../../../components/buildStages/StageWaterfall'
import {
  useBuildStages,
  useRefetchOnBuildSteps,
} from '../../../../../components/buildStages/useBuildStages'
import {
  useRunLiveFeedback,
  type RunLiveState,
} from '../../../../../hooks/useRunLiveFeedback'
import { goToSection } from '../../../../../lib/runSections'
import {
  isBuildStageName,
  type BuildStageName,
} from '../../../../../lib/buildStages'

// Lazy so the report table stays out of the deploy detail chunk until the build tab opens.
const BuildReportSection = lazy(() =>
  import('../../../../../components/deployDetail/BuildReportSection').then(
    (m) => ({ default: m.BuildReportSection }),
  ),
)

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
  const [stageFilter, setStageFilter] = useState<BuildStageName | null>(null)
  const { data: stageData } = useBuildStages(name, deployId)
  useRefetchOnBuildSteps(name, deployId, steps)
  const liveState: RunLiveState =
    attempt?.status === 'running' || attempt?.status === 'queued'
      ? 'running'
      : attempt?.status === 'failed'
        ? 'failed'
        : attempt?.status === 'succeeded'
          ? 'done'
          : 'idle'
  useRunLiveFeedback(liveState, {
    running: t('run.titleRunning'),
    failed: t('run.titleFailed'),
    doneToast: t('run.doneToast'),
    failedToast: t('run.failedToast'),
  })

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

  const rolloutTabStage =
    isStatic && state.phase === 'live'
      ? { ...stages[1], status: 'done' as const }
      : stages[1]
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
          goToSection('logs')
        }}
      />
      <DeployExplainPanel
        deployId={deployId}
        failed={attempt.status === 'failed'}
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

      <DeployPhases
        deployId={deployId}
        running={!attemptFinished}
        lastEventAt={lastEventAt}
        streamHealthy={transport === 'sse' && connectionState === 'open'}
        latestLine={lines.at(-1)?.line ?? null}
      />

      <StreamDropBanner
        transport={transport}
        connectionState={connectionState}
        retryAt={retryAt}
        onRetry={retry}
        onSwitchToPolling={switchToPolling}
        downloadUrl={deployLogDownloadURL(name, deployId)}
      />

      <StageOverview
        appName={name}
        deployId={deployId}
        selected={stageFilter}
        onSelect={(stage) => {
          setStageFilter(stage)
          goToSection('logs')
        }}
      />

      <RunSectionNav />

      <div className="flex flex-col gap-4">
        <RunSection
          id="summary"
          title={t('sections.summary')}
          chip={attempt.commit_sha?.slice(0, 7)}
        >
          <BuildTab
            part="summary"
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
          <SupplyChainSection
            appName={name}
            deploymentId={deployId}
            sbomPackages={attempt.sbom_packages}
          />
          <SecretScanSection appName={name} deploymentId={deployId} />
        </RunSection>

        <RunSection
          id="waterfall"
          title={t('sections.waterfall')}
          chip={
            stageData
              ? t('run.stageCount', { count: stageData.stages.length })
              : undefined
          }
        >
          {stageData && stageData.stages.length > 0 ? (
            <StageWaterfall data={stageData} nowMs={nowMs} />
          ) : (
            <p className="text-sm text-muted-foreground">
              {t('buildStages.empty')}
            </p>
          )}
        </RunSection>

        <RunSection
          id="rollout"
          title={t('sections.rollout')}
          chip={t(
            `buildStages.status.${rolloutTabStage.status === 'failed' ? 'failed' : rolloutTabStage.status === 'done' ? 'done' : 'running'}`,
          )}
        >
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
        </RunSection>

        <RunSection
          id="logs"
          title={t('sections.logs')}
          chip={lines.length > 0 ? lines.length.toLocaleString() : undefined}
        >
          <div
            id={deployStageAnchorId('build')}
            tabIndex={-1}
            className="space-y-3 outline-none"
          >
            <div className="flex flex-wrap items-center justify-between gap-2">
              <label className="flex items-center gap-2 text-xs text-muted-foreground">
                {t('run.stageFilter')}
                <select
                  value={stageFilter ?? ''}
                  onChange={(e) => {
                    const v = e.target.value
                    setStageFilter(isBuildStageName(v) ? v : null)
                  }}
                  className="rounded-md border border-border bg-background px-2 py-1 text-sm text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                >
                  <option value="">{t('run.allStages')}</option>
                  {(stageData?.stages ?? []).map((s) => (
                    <option key={s.stage} value={s.stage}>
                      {t(`buildStages.stage.${s.stage}`)}
                    </option>
                  ))}
                </select>
              </label>
              <DeployLiveStatus
                transport={transport}
                connectionState={connectionState}
                lastEventAt={lastEventAt}
                retryAt={retryAt}
                onRetry={retry}
              />
            </div>
            {stageFilter ? (
              <StageLogPanel
                key={stageFilter}
                appName={name}
                deployId={deployId}
                stage={stageFilter}
                finished={attemptFinished}
              />
            ) : (
              <BuildTab
                part="logs"
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
            )}
            {attemptFinished ? (
              <Suspense fallback={null}>
                <BuildReportSection appName={name} deployId={deployId} />
              </Suspense>
            ) : null}
          </div>
        </RunSection>

        <RunSection
          id="compare"
          title={t('sections.compare')}
          defaultOpen={false}
        >
          <p className="text-sm text-muted-foreground">
            {t('run.compareBody')}
          </p>
          <Button
            variant="outline"
            size="sm"
            nativeButton={false}
            render={
              <Link
                to="/apps/$name/deploys/compare"
                params={{ name }}
                search={{ from: attempt.id }}
              />
            }
          >
            {t('run.compareCta')}
          </Button>
        </RunSection>
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
