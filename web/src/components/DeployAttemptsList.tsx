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
import type { DeployAttempt, DeployAttemptStatus } from '../types/deployAttempt'
import { useTriggerDeploy } from '../queries/deploys'
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
export function DeployAttemptsList({
  appName,
  attempts,
}: {
  appName: string
  attempts: DeployAttempt[]
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

  return (
    <Card>
      <CardHeader>
        <CardTitle>Deploy history</CardTitle>
      </CardHeader>
      <CardContent>
        <ul className="-mx-4 divide-y divide-border">
          {attempts.map((a) => (
            <DeployAttemptRow key={a.id} appName={appName} attempt={a} />
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}

function DeployAttemptRow({
  appName,
  attempt,
}: {
  appName: string
  attempt: DeployAttempt
}) {
  const triggerDeploy = useTriggerDeploy(appName)
  const StatusIcon = STATUS_ICON[attempt.status]

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
          className={attempt.status === 'running' ? 'size-3 animate-spin' : 'size-3'}
        />
        {STATUS_LABEL[attempt.status]}
      </Badge>

      <div className="min-w-0 flex-1">
        <p className="truncate font-mono text-sm font-medium text-foreground">
          {attempt.image}
        </p>
        <p className="mt-0.5 text-xs text-muted-foreground/70">
          Started {new Date(attempt.started_at).toLocaleString()}
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
            <ArrowCounterClockwiseIcon className="size-3.5" data-icon="inline-start" />
            {triggerDeploy.isPending ? 'Rolling back...' : 'Rollback to this build'}
          </Button>
        ) : null}
      </div>
    </li>
  )
}
