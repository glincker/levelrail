import { Display } from '@/components/ui/typography'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { StatusPill } from '@/components/kit'
import type { AppDetail } from '../../types/appDetail'
import type { ReconcileCondition } from '../../types/deploy'
import type { DeployAttempt } from '../../types/deployAttempt'
import { computeDeployStages } from '../../lib/deployStages'
import { deriveHeroStatus, phaseWords } from './heroStatus'
import { HeroActions } from './HeroActions'
import { pickHeroPrimary } from './heroPrimary'
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
  gitMissing = false,
}: {
  app: AppDetail
  conditions: ReconcileCondition[]
  latestAttempt?: DeployAttempt
  attempts?: DeployAttempt[]
  url: string | null
  attentionCount?: number
  serverOutcome?: { outcome?: string; reason?: string }
  onDeploy: (tab: DeployTab) => void
  /** The git source query answered 404: offer the connect flow. */
  gitMissing?: boolean
}) {
  const { t } = useTranslation('common')
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

  const primary = pickHeroPrimary({
    suspended: app.suspended,
    latestStatus: latestAttempt?.status,
  })

  return (
    <>
      <section
        aria-label={t('overviewHero.stickyLabel')}
        className="sticky top-0 z-20 flex items-center justify-between gap-2 rounded-[var(--glinui-radius-lg)] border border-border bg-card px-3 py-2 shadow-sm"
      >
        <div className="flex min-w-0 items-center gap-x-3">
          <Display as="h1" size="page" className="truncate">
            {app.name}
          </Display>
          <StatusPill
            tone={status.tone}
            label={status.label}
            live={status.live}
            title={status.title}
          />
          <span className="max-sm:hidden">
            <AppLiveBadge app={app.name} />
          </span>
          {runningStage && latestAttempt ? (
            <Link
              to="/apps/$name/deploys/$deployId/logs"
              params={{ name: app.name, deployId: latestAttempt.id }}
              className="text-xs whitespace-nowrap text-primary underline underline-offset-2 max-sm:hidden"
            >
              Watch live
            </Link>
          ) : null}
        </div>
        <HeroActions
          app={app}
          url={url}
          onDeploy={onDeploy}
          primary={primary}
          gitMissing={gitMissing}
        />
      </section>
      <section
        aria-label="App summary"
        className="flex flex-wrap items-center gap-x-4 gap-y-1 px-1"
      >
        <div className="flex min-w-0 items-center gap-0.5">
          <UrlLine appName={app.name} url={url} />
          <AutoUrlInfo appName={app.name} />
        </div>
        <ReleaseLine app={app} latest={released ?? latestAttempt} />
        <TagChips appName={app.name} tags={app.tags} />
        <AppHeaderBadges app={app} attentionCount={attentionCount} />
      </section>
    </>
  )
}
