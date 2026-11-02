import { Display } from '@/components/ui/typography'
import { Link } from '@tanstack/react-router'
import { StatusPill } from '@/components/kit'
import type { AppDetail } from '../../types/appDetail'
import type { ReconcileCondition } from '../../types/deploy'
import type { DeployAttempt } from '../../types/deployAttempt'
import { computeDeployStages } from '../../lib/deployStages'
import { deriveHeroStatus, phaseWords } from './heroStatus'
import { HeroActions } from './HeroActions'
import { ReleaseLine } from './ReleaseLine'
import { TagChips } from './TagChips'
import { AutoUrlInfo } from './AutoUrlInfo'
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
  serverOutcome,
  onDeploy,
}: {
  app: AppDetail
  conditions: ReconcileCondition[]
  latestAttempt?: DeployAttempt
  attempts?: DeployAttempt[]
  url: string | null
  attentionCount?: number
  serverOutcome?: { outcome?: string; reason?: string }
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
    serverOutcome,
  })

  const released =
    attempts.find(
      (a) => a.status === 'succeeded' && a.rollout_state === 'serving',
    ) ?? attempts.find((a) => a.status === 'succeeded')

  return (
    <section
      aria-label="App summary"
      className="flex flex-col gap-2 rounded-[var(--glinui-radius-lg)] border border-border bg-card px-4 py-3 lg:flex-row lg:items-center lg:justify-between lg:gap-4"
    >
      <div className="min-w-0 space-y-1.5">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <Display as="h1" size="page" className="truncate">
            {app.name}
          </Display>
          <StatusPill
            tone={status.tone}
            label={status.label}
            live={status.live}
            title={status.title}
          />
          <AppLiveBadge app={app.name} />
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
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1">
          <div className="flex min-w-0 items-center gap-0.5">
            <UrlLine appName={app.name} url={url} />
            <AutoUrlInfo appName={app.name} />
          </div>
          <ReleaseLine app={app} latest={released ?? latestAttempt} />
        </div>
        <TagChips appName={app.name} tags={app.tags} />
      </div>
      <div className="flex shrink-0 flex-wrap items-center gap-2 lg:justify-end">
        <AppHeaderBadges app={app} attentionCount={attentionCount} />
        <HeroActions app={app} url={url} onDeploy={onDeploy} />
      </div>
    </section>
  )
}
