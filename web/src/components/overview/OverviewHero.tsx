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
import { UrlLine } from './UrlLine'
import type { DeployTab } from './DeploySheet'

export function OverviewHero({
  app,
  conditions,
  latestAttempt,
  url,
  onDeploy,
}: {
  app: AppDetail
  conditions: ReconcileCondition[]
  latestAttempt?: DeployAttempt
  url: string | null
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

  return (
    <section
      aria-label="App summary"
      className="flex flex-col gap-4 rounded-2xl border border-border bg-card p-4 sm:p-6 lg:flex-row lg:items-start lg:justify-between"
    >
      <div className="min-w-0 space-y-3">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <h1 className="truncate text-2xl font-semibold text-foreground">
            {app.name}
          </h1>
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
        <ReleaseLine app={app} latest={latestAttempt} />
        <TagChips appName={app.name} tags={app.tags} />
      </div>
      <HeroActions app={app} url={url} onDeploy={onDeploy} />
    </section>
  )
}
