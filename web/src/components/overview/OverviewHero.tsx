import { Display } from '@/components/ui/typography'
import { Link } from '@tanstack/react-router'
import { StatusPill } from '@/components/kit'
import type { AppDetail } from '../../types/appDetail'
import type { ReconcileCondition } from '../../types/deploy'
import type { DeployAttempt } from '../../types/deployAttempt'
import { computeDeployStages } from '../../lib/deployStages'
import { deriveHeroStatus, phaseWords } from './heroStatus'
import { DeployPreviewThumb } from '../DeployPreviewThumb'
import { HeroActions } from './HeroActions'
import { ReleaseLine } from './ReleaseLine'
import { TagChips } from './TagChips'
import { AutoUrlTlsNote } from './AutoUrlTlsNote'
import { AutoUrlBetaNote } from './AutoUrlBetaNote'
import { UrlLine } from './UrlLine'
import { AppLiveBadge } from '../golive/GoLiveBadge'
import { AppHeaderBadges } from '../apps/AppHeaderBadges'
import type { DeployTab } from './DeploySheet'

export function OverviewHero({
  app,
  conditions,
  latestAttempt,
  attempts = [],
  url,
  attentionCount = 0,
  onDeploy,
}: {
  app: AppDetail
  conditions: ReconcileCondition[]
  latestAttempt?: DeployAttempt
  attempts?: DeployAttempt[]
  url: string | null
  attentionCount?: number
  onDeploy: (tab: DeployTab) => void
}) {
  const runningStage = latestAttempt
    ? computeDeployStages(latestAttempt, conditions, true).find(
        (s) => s.status === 'running',
      )
    : undefined
  const status = deriveHeroStatus({
    conditions,
    suspended: app.suspended,
    envDirty: app.env_dirty,
    deploying: runningStage !== undefined,
    deployPhase: phaseWords(runningStage?.key),
    latestAttemptStatus: latestAttempt?.status,
  })

  const released =
    attempts.find(
      (a) => a.status === 'succeeded' && a.rollout_state === 'serving',
    ) ?? attempts.find((a) => a.status === 'succeeded')

  return (
    <section
      aria-label="App summary"
      className="flex flex-col gap-4 rounded-[var(--glinui-radius-lg)] border border-border bg-card p-4 lg:flex-row lg:items-start lg:justify-between"
    >
      <div className="min-w-0 space-y-3">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <Display as="h1" size="page" className="truncate">
            {app.name}
          </Display>
          <StatusPill
            tone={status.tone}
            label={status.label}
            live={status.live}
            title={status.title}
          />
          {runningStage && latestAttempt ? (
            <Link
              to="/apps/$name/deploys/$deployId/logs"
              params={{ name: app.name, deployId: latestAttempt.id }}
              className="text-xs text-primary underline underline-offset-2"
            >
              Watch live
            </Link>
          ) : null}
        </div>
        <UrlLine appName={app.name} url={url} />
        <AppLiveBadge app={app.name} />
        <AutoUrlBetaNote appName={app.name} />
        <AutoUrlTlsNote appName={app.name} />
        <ReleaseLine app={app} latest={released ?? latestAttempt} />
        <TagChips appName={app.name} tags={app.tags} />
      </div>
      <div className="flex shrink-0 flex-col gap-3 lg:items-end">
        <AppHeaderBadges app={app} attentionCount={attentionCount} />
        <HeroActions app={app} url={url} onDeploy={onDeploy} />
        {released ? (
          <DeployPreviewThumb
            appName={app.name}
            deploymentId={released.id}
            imageUrl={released.preview_image_url}
            commitSha={released.commit_sha}
            canRecapture
            size="md"
            hideWhenEmpty
          />
        ) : null}
      </div>
    </section>
  )
}
