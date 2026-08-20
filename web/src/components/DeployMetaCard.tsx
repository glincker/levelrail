import type { ReactNode } from 'react'
import {
  CheckCircleIcon,
  ClockIcon,
  GitBranchIcon,
  GitCommitIcon,
  GlobeIcon,
  LightningIcon,
  SpinnerGapIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { Link } from '@tanstack/react-router'
import type { VariantProps } from 'class-variance-authority'
import type { DeployAttempt } from '../types/deployAttempt'
import type { DeployStage } from '../lib/deployStages'
import {
  DEPLOY_ATTEMPT_SOURCE_LABEL,
  DEPLOY_ATTEMPT_STATUS_BADGE_VARIANT,
  DEPLOY_ATTEMPT_STATUS_ICON,
  DEPLOY_ATTEMPT_STATUS_LABEL,
} from '../lib/deployAttemptPresentation'
import { formatDurationMs } from '../lib/deployDuration'
import { useNowTick } from '../hooks/useNowTick'
import { Card, CardContent } from '@/components/ui/card'
import { Badge, type badgeVariants } from '@/components/ui/badge'

type BadgeVariant = VariantProps<typeof badgeVariants>['variant']

interface OverallStatus {
  label: string
  icon: Icon
  badgeVariant: BadgeVariant
  spinning: boolean
}

// Combines the two real stages (see lib/deployStages.ts) into one
// headline word, the way the reference layout's status cell reads
// "Ready"/"Building"/"Error" rather than a raw attempt status: purely a
// display-layer synthesis of data already computed, no new signal.
function deriveOverallStatus(
  attempt: DeployAttempt,
  stages: [DeployStage, DeployStage],
): OverallStatus {
  const [build, rollout] = stages
  if (build.status === 'running') {
    return {
      label: 'Building',
      icon: SpinnerGapIcon,
      badgeVariant: 'muted',
      spinning: true,
    }
  }
  if (build.status === 'failed') {
    return {
      label: 'Build failed',
      icon: WarningCircleIcon,
      badgeVariant: 'destructive',
      spinning: false,
    }
  }
  if (rollout.status === 'running' || rollout.status === 'pending') {
    return {
      label: 'Rolling out',
      icon: SpinnerGapIcon,
      badgeVariant: 'muted',
      spinning: true,
    }
  }
  if (rollout.status === 'failed') {
    return {
      label: 'Roll out failed',
      icon: WarningCircleIcon,
      badgeVariant: 'destructive',
      spinning: false,
    }
  }
  if (rollout.status === 'done') {
    return {
      label: 'Ready',
      icon: CheckCircleIcon,
      badgeVariant: 'success',
      spinning: false,
    }
  }
  // rollout is 'unknown' (a past, non-latest attempt, see
  // computeDeployStages) or the build was 'skipped' (image source) with
  // no rollout signal yet: fall back to the attempt's own status.
  return {
    label: DEPLOY_ATTEMPT_STATUS_LABEL[attempt.status],
    icon: DEPLOY_ATTEMPT_STATUS_ICON[attempt.status],
    badgeVariant: DEPLOY_ATTEMPT_STATUS_BADGE_VARIANT[attempt.status],
    spinning: attempt.status === 'running',
  }
}

// The overall deploy's finished timestamp, once genuinely known: rollout
// finishing (done or failed) ends it; a build failure ends it without a
// rollout ever starting; a not-latest attempt's untracked rollout falls
// back to the build's own finish time, the latest bound this codebase can
// honestly state for it. undefined means still in flight.
function overallFinishedAt(
  attempt: DeployAttempt,
  stages: [DeployStage, DeployStage],
): string | undefined {
  const [build, rollout] = stages
  if (rollout.finishedAt) return rollout.finishedAt
  if (build.status === 'failed') return attempt.finished_at
  if (rollout.status === 'unknown' || rollout.status === 'skipped') {
    return attempt.finished_at
  }
  return undefined
}

function useOverallDuration(
  attempt: DeployAttempt,
  stages: [DeployStage, DeployStage],
): string {
  const finishedAt = overallFinishedAt(attempt, stages)
  const now = useNowTick(!finishedAt)
  const startMs = new Date(attempt.started_at).getTime()
  const endMs = finishedAt ? new Date(finishedAt).getTime() : now
  return formatDurationMs(endMs - startMs) ?? 'Unknown'
}

// The "Deployment Details" metadata grid: status, live duration,
// trigger, when it started, the domain(s) it's reachable at, and the
// exact source it built from. Branch is shown only for a webhook-
// triggered attempt: that's the one case a connected git source's
// configured branch is provably the branch this attempt built from (see
// internal/webhook's target-branch match); a manual build accepts an
// arbitrary caller-supplied ref that is never persisted, so showing the
// configured branch there could be wrong.
export function DeployMetaCard({
  attempt,
  stages,
  domains,
  branch,
}: {
  attempt: DeployAttempt
  stages: [DeployStage, DeployStage]
  domains?: string[]
  branch?: string
}) {
  const status = deriveOverallStatus(attempt, stages)
  const StatusIcon = status.icon
  const duration = useOverallDuration(attempt, stages)

  return (
    <Card>
      <CardContent>
        <dl className="grid grid-cols-2 gap-x-4 gap-y-4 sm:grid-cols-3">
          <MetaField label="Status">
            <Badge variant={status.badgeVariant} className="rounded-full">
              <StatusIcon
                className={status.spinning ? 'size-3 animate-spin' : 'size-3'}
              />
              {status.label}
            </Badge>
          </MetaField>

          <MetaField label="Duration">
            <span className="flex items-center gap-1.5 text-sm text-foreground">
              <ClockIcon
                className="size-3.5 text-muted-foreground"
                aria-hidden="true"
              />
              {duration}
            </span>
          </MetaField>

          <MetaField label="Trigger">
            <span className="flex items-center gap-1.5 text-sm text-foreground">
              <LightningIcon
                className="size-3.5 text-muted-foreground"
                aria-hidden="true"
              />
              {attempt.source
                ? DEPLOY_ATTEMPT_SOURCE_LABEL[attempt.source]
                : 'Unknown'}
            </span>
          </MetaField>

          <MetaField label="Started">
            <span className="text-sm text-foreground">
              {new Date(attempt.started_at).toLocaleString()}
            </span>
          </MetaField>

          <MetaField label="Domains">
            {domains && domains.length > 0 ? (
              <div className="flex flex-wrap items-center gap-1.5">
                {domains.map((d) => (
                  <span
                    key={d}
                    className="flex items-center gap-1 rounded bg-muted px-1.5 py-0.5 text-xs text-foreground"
                  >
                    <GlobeIcon
                      className="size-3 text-muted-foreground"
                      aria-hidden="true"
                    />
                    {d}
                  </span>
                ))}
              </div>
            ) : (
              <Link
                to="/apps/$name/domains"
                params={{ name: attempt.service_name }}
                className="text-sm text-muted-foreground underline underline-offset-2"
              >
                No domain configured
              </Link>
            )}
          </MetaField>

          <MetaField label="Source">
            <div className="flex flex-wrap items-center gap-1.5">
              <span className="truncate rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-foreground">
                {attempt.image}
              </span>
              {attempt.commit_sha ? (
                <span
                  className="flex items-center gap-1 rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-foreground"
                  title={attempt.commit_sha}
                >
                  <GitCommitIcon
                    className="size-3 text-muted-foreground"
                    aria-hidden="true"
                  />
                  {attempt.commit_sha.slice(0, 7)}
                </span>
              ) : null}
              {branch ? (
                <span className="flex items-center gap-1 rounded bg-muted px-1.5 py-0.5 text-xs text-foreground">
                  <GitBranchIcon
                    className="size-3 text-muted-foreground"
                    aria-hidden="true"
                  />
                  {branch}
                </span>
              ) : null}
            </div>
          </MetaField>
        </dl>
      </CardContent>
    </Card>
  )
}

function MetaField({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <div className="min-w-0">
      <dt className="text-xs font-medium text-muted-foreground uppercase tracking-wide">
        {label}
      </dt>
      <dd className="mt-1">{children}</dd>
    </div>
  )
}
