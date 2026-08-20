import { Link } from '@tanstack/react-router'
import {
  CheckCircleIcon,
  WarningCircleIcon,
  SpinnerGapIcon,
  ScrollIcon,
  ArrowCounterClockwiseIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import type { VariantProps } from 'class-variance-authority'
import type {
  DeployAttempt,
  DeployAttemptSource,
  DeployAttemptStatus,
} from '../types/deployAttempt'
import type { ReconcileCondition } from '../types/deploy'
import { useTriggerDeploy } from '../queries/deploys'
import { formatDeployDuration } from '../lib/deployDuration'
import { computeDeployStages } from '../lib/deployStages'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'

const STATUS_BADGE_VARIANT: Record<
  DeployAttemptStatus,
  VariantProps<typeof badgeVariants>['variant']
> = {
  succeeded: 'success',
  failed: 'destructive',
  running: 'muted',
}

const STATUS_ICON: Record<DeployAttemptStatus, Icon> = {
  succeeded: CheckCircleIcon,
  failed: WarningCircleIcon,
  running: SpinnerGapIcon,
}

const STATUS_LABEL: Record<DeployAttemptStatus, string> = {
  succeeded: 'Succeeded',
  failed: 'Failed',
  running: 'Running',
}

const SOURCE_LABEL: Record<DeployAttemptSource, string> = {
  webhook: 'Webhook',
  manual: 'Manual build',
  image: 'Image',
}

// DeployAttemptsList renders GET /api/v1/apps/{name}/deploy-attempts'
// real, row-per-attempt history (internal/api/deploy_attempts.go), the
// list this task's own design note exists to unblock: a status, an
// image tag, a timestamp, a link into the already-existing (previously
// orphaned) SSE log viewer route, and, for a past successful attempt, a
// "Rollback to this build" action. Rollback needs no dedicated backend
// endpoint: it is exactly POST /api/v1/apps/{name}/deploys
// (useTriggerDeploy, the same mutation DeployTriggerForm's "existing
// image" tab already calls) given that attempt's own Image, per
// handleTriggerDeploy's own doc comment framing a redeploy to an older
// tag as identical to one to a newer tag.
//
// Newest-first, no pagination: matches ListDeployAttempts' own documented
// scope (a known, deliberately deferred follow-up if a single app's
// history grows very large), and this list is not virtualized for the
// same reason AppRow's own list is expected to stay well under the
// virtualization threshold in realistic usage; revisit both together if
// that assumption stops holding.
//
// conditions is optional and only used for the latest attempt's badge
// (which stage is running or failed, see lib/deployStages.ts): older
// attempts have no reconcile-status signal to draw on.
export function DeployAttemptsList({
  appName,
  attempts,
  conditions = [],
}: {
  appName: string
  attempts: DeployAttempt[]
  conditions?: ReconcileCondition[]
}) {
  if (attempts.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Deploy history</CardTitle>
        </CardHeader>
        <CardContent className="text-sm text-muted-foreground">
          No deploys triggered yet.
        </CardContent>
      </Card>
    )
  }

  const latestAttemptId = attempts[0]?.id

  return (
    <Card>
      <CardHeader>
        <CardTitle>Deploy history</CardTitle>
      </CardHeader>
      <CardContent>
        <ul className="-mx-4 divide-y divide-border">
          {attempts.map((a) => (
            <DeployAttemptRow
              key={a.id}
              appName={appName}
              attempt={a}
              isLatestAttempt={a.id === latestAttemptId}
              conditions={conditions}
            />
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}

function DeployAttemptRow({
  appName,
  attempt,
  isLatestAttempt,
  conditions,
}: {
  appName: string
  attempt: DeployAttempt
  isLatestAttempt: boolean
  conditions: ReconcileCondition[]
}) {
  const triggerDeploy = useTriggerDeploy(appName)
  const StatusIcon = STATUS_ICON[attempt.status]
  // Stage vocabulary (Build/Roll out) reuses computeDeployStages, the
  // same derivation the live deploy view uses: only meaningful for the
  // latest attempt, since reconcile conditions carry no per-attempt
  // history (see that function's own doc comment). Failed shows which
  // stage failed; running shows which stage is currently in flight.
  const stageLabel = isLatestAttempt
    ? computeDeployStages(attempt, conditions, true).find((s) =>
        attempt.status === 'failed' ? s.status === 'failed' : s.status === 'running',
      )?.label
    : undefined

  const handleRollback = () => {
    triggerDeploy.mutate(attempt.image, {
      onSuccess: () => {
        toast.add({
          title: 'Rollback triggered.',
          description: `Redeploying ${attempt.image}.`,
          type: 'success',
        })
      },
    })
  }

  return (
    <li className="flex items-start gap-3 px-4 py-3 first:pt-0 last:pb-0">
      <Badge
        variant={STATUS_BADGE_VARIANT[attempt.status]}
        className="mt-0.5 shrink-0 rounded-full"
      >
        <StatusIcon
          className={
            attempt.status === 'running' ? 'size-3 animate-spin' : 'size-3'
          }
        />
        {STATUS_LABEL[attempt.status]}
        {stageLabel ? ` (${stageLabel})` : ''}
      </Badge>

      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <p className="truncate font-mono text-sm font-medium text-foreground">
            {attempt.image}
          </p>
          {attempt.commit_sha ? (
            <span className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs text-muted-foreground">
              {attempt.commit_sha.slice(0, 7)}
            </span>
          ) : null}
          {attempt.source ? (
            <Badge variant="outline" className="shrink-0">
              {SOURCE_LABEL[attempt.source]}
            </Badge>
          ) : null}
        </div>
        <p className="mt-0.5 text-xs text-muted-foreground/70">
          Started {new Date(attempt.started_at).toLocaleString()} ·{' '}
          {formatDeployDuration(attempt.started_at, attempt.finished_at)}
        </p>
        {attempt.error ? (
          <p className="mt-1 text-xs text-destructive">{attempt.error}</p>
        ) : null}
      </div>

      <div className="flex shrink-0 items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          nativeButton={false}
          render={
            <Link
              to="/apps/$name/deploys/$deployId/logs"
              params={{ name: appName, deployId: attempt.id }}
            />
          }
        >
          <ScrollIcon className="size-3.5" data-icon="inline-start" />
          Logs
        </Button>
        {attempt.status === 'succeeded' ? (
          <Button
            variant="outline"
            size="sm"
            onClick={handleRollback}
            disabled={triggerDeploy.isPending}
          >
            <ArrowCounterClockwiseIcon
              className="size-3.5"
              data-icon="inline-start"
            />
            {triggerDeploy.isPending
              ? 'Rolling back...'
              : 'Rollback to this build'}
          </Button>
        ) : null}
      </div>
    </li>
  )
}
