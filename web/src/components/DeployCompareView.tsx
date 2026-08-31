import {
  ArrowRightIcon,
  ArrowCounterClockwiseIcon,
  InfoIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { DeployCompare, DeployCompareSide } from '../types/deployCompare'
import { useTriggerDeploy } from '../queries/deploys'
import { formatDeployDuration } from '../lib/deployDuration'
import { DEPLOY_ATTEMPT_SOURCE_LABEL } from '../lib/deployAttemptPresentation'
import type { DeployAttemptSource } from '../types/deployAttempt'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { toast } from '@/components/ui/toast'

// Human labels for deployCompareResource.Changes[].Field
// (internal/api/deploy_compare.go's diffDeployCompareSides), the wire
// vocabulary this component renders.
const CHANGE_FIELD_LABEL: Record<string, string> = {
  image: 'Image',
  commit_sha: 'Commit',
  source: 'Trigger source',
}

// DeployCompareView renders GET .../deploys/compare's before/after diff:
// two side cards (each with its own "Roll back to this" CTA, distinct
// from the deploy history list's own rollback button, per this task's
// own requirement), the fields that actually differ, and an explicit
// note about what deploy_attempts never snapshotted rather than a
// fabricated diff for data that was never captured.
export function DeployCompareView({
  appName,
  compare,
}: {
  appName: string
  compare: DeployCompare
}) {
  return (
    <div className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <DeployCompareSideCard appName={appName} label="From" side={compare.from} />
        <DeployCompareSideCard appName={appName} label="To" side={compare.to} />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>What changed</CardTitle>
        </CardHeader>
        <CardContent>
          {compare.changes.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No tracked fields differ between these two deploys.
            </p>
          ) : (
            <ul className="space-y-2">
              {compare.changes.map((c) => (
                <li
                  key={c.field}
                  className="flex flex-wrap items-center gap-2 text-sm"
                >
                  <span className="w-32 shrink-0 font-medium text-foreground">
                    {CHANGE_FIELD_LABEL[c.field] ?? c.field}
                  </span>
                  <span className="truncate font-mono text-xs text-muted-foreground">
                    {c.from || '(none)'}
                  </span>
                  <ArrowRightIcon
                    className="size-3.5 shrink-0 text-muted-foreground"
                    aria-hidden="true"
                  />
                  <span className="truncate font-mono text-xs text-foreground">
                    {c.to || '(none)'}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>

      <Alert>
        <InfoIcon className="size-4" />
        <AlertDescription>
          <p>{compare.note}</p>
          <p className="mt-1.5 font-mono text-xs opacity-80">
            Not tracked: {compare.unsnapshotted_fields.join(', ')}
          </p>
        </AlertDescription>
      </Alert>
    </div>
  )
}

function DeployCompareSideCard({
  appName,
  label,
  side,
}: {
  appName: string
  label: string
  side: DeployCompareSide
}) {
  const triggerDeploy = useTriggerDeploy(appName)

  const handleRollback = () => {
    triggerDeploy.mutate(side.image, {
      onSuccess: () => {
        toast.add({
          title: 'Rollback triggered.',
          description: `Redeploying ${side.image}.`,
          type: 'success',
        })
      },
    })
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
        <CardTitle className="text-sm text-muted-foreground">
          {label}
        </CardTitle>
        {side.is_current ? (
          <Badge variant="outline">Currently running</Badge>
        ) : side.status ? (
          <Badge variant="outline">{side.status}</Badge>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-2">
        <p className="truncate font-mono text-sm font-medium text-foreground">
          {side.image}
        </p>
        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          {side.commit_sha ? (
            <span className="rounded bg-muted px-1.5 py-0.5 font-mono">
              {side.commit_sha.slice(0, 7)}
            </span>
          ) : null}
          {side.source ? (
            <Badge variant="outline" className="shrink-0">
              {DEPLOY_ATTEMPT_SOURCE_LABEL[side.source as DeployAttemptSource] ??
                side.source}
            </Badge>
          ) : null}
        </div>
        {side.started_at ? (
          <p className="text-xs text-muted-foreground/70">
            Started {new Date(side.started_at).toLocaleString()}
            {side.finished_at
              ? ` · ${formatDeployDuration(side.started_at, side.finished_at)}`
              : ''}
          </p>
        ) : null}

        {!side.is_current ? (
          <Button
            variant="outline"
            size="sm"
            className="mt-1"
            onClick={handleRollback}
            disabled={triggerDeploy.isPending}
          >
            <ArrowCounterClockwiseIcon
              className="size-3.5"
              data-icon="inline-start"
            />
            {triggerDeploy.isPending ? 'Rolling back...' : 'Roll back to this build'}
          </Button>
        ) : null}
      </CardContent>
    </Card>
  )
}
