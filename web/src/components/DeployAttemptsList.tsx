import { Link } from '@tanstack/react-router'
import { useState } from 'react'
import {
  ScrollIcon,
  ArrowCounterClockwiseIcon,
  RocketLaunchIcon,
  GitDiffIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { DeployAttempt } from '../types/deployAttempt'
import type { ReconcileCondition } from '../types/deploy'
import { useTriggerDeploy } from '../queries/deploys'
import { formatDeployDuration } from '../lib/deployDuration'
import { computeDeployStages } from '../lib/deployStages'
import {
  DEPLOY_ATTEMPT_SOURCE_LABEL,
  DEPLOY_ATTEMPT_STATUS_BADGE_VARIANT,
  DEPLOY_ATTEMPT_STATUS_ICON,
  DEPLOY_ATTEMPT_STATUS_LABEL,
} from '../lib/deployAttemptPresentation'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { EmptyState } from '@/components/ui/empty-state'
import { toast } from '@/components/ui/toast'

// Scrolls to the "Trigger a deploy" card pinned above this list on the
// same route (see routes/apps/$name.tsx's showDeployTrigger) rather than
// duplicating that form here, then moves focus into its image-tag field
// (falling back to the tab buttons above it) so a keyboard user lands
// somewhere useful instead of just visually nearby.
function focusDeployTriggerForm() {
  const form = document.getElementById('deploy-trigger-form')
  form?.scrollIntoView({ behavior: 'smooth', block: 'start' })
  const target = form?.querySelector<HTMLElement>('input') ?? form?.querySelector<HTMLElement>('button')
  target?.focus()
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
  // Capped at 2: a comparison is always between exactly two points (or
  // one point and "current", the per-row Compare-to-current button
  // below), so a third pick replaces the oldest selection rather than
  // growing an N-way comparison this feature doesn't support. Declared
  // before the empty-state early return below so hook order stays fixed
  // regardless of attempts.length.
  const [selected, setSelected] = useState<string[]>([])

  if (attempts.length === 0) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Deploy history</CardTitle>
        </CardHeader>
        <CardContent>
          <EmptyState
            icon={<RocketLaunchIcon className="size-5" />}
            title="No deploys yet"
            description="Trigger your first deploy above, either from an existing image tag or by building from a git source."
            action={
              <Button size="sm" onClick={focusDeployTriggerForm}>
                <RocketLaunchIcon className="size-3.5" data-icon="inline-start" />
                Go to deploy form
              </Button>
            }
          />
        </CardContent>
      </Card>
    )
  }

  const latestAttemptId = attempts[0]?.id

  const toggleSelected = (id: string) => {
    setSelected((prev) => {
      if (prev.includes(id)) return prev.filter((x) => x !== id)
      if (prev.length < 2) return [...prev, id]
      return [...prev.slice(1), id]
    })
  }

  // Ordered oldest-first so "from" always reads as the earlier deploy
  // regardless of the order the two rows were clicked in.
  const compareSelectedSearch = (() => {
    if (selected.length !== 2) return null
    const [a, b] = selected
      .map((id) => attempts.find((x) => x.id === id))
      .filter((x): x is DeployAttempt => x !== undefined)
      .sort((x, y) => new Date(x.started_at).getTime() - new Date(y.started_at).getTime())
    if (!a || !b) return null
    return { from: a.id, to: b.id }
  })()

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-2 space-y-0">
        <CardTitle>Deploy history</CardTitle>
        {compareSelectedSearch ? (
          <Button
            size="sm"
            nativeButton={false}
            render={
              <Link
                to="/apps/$name/deploys/compare"
                params={{ name: appName }}
                search={compareSelectedSearch}
              />
            }
          >
            <GitDiffIcon className="size-3.5" data-icon="inline-start" />
            Compare selected
          </Button>
        ) : selected.length === 1 ? (
          <p className="text-xs text-muted-foreground">
            Select one more deploy to compare
          </p>
        ) : null}
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
              isSelected={selected.includes(a.id)}
              onToggleSelected={() => toggleSelected(a.id)}
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
  isSelected,
  onToggleSelected,
}: {
  appName: string
  attempt: DeployAttempt
  isLatestAttempt: boolean
  conditions: ReconcileCondition[]
  isSelected: boolean
  onToggleSelected: () => void
}) {
  const triggerDeploy = useTriggerDeploy(appName)
  const StatusIcon = DEPLOY_ATTEMPT_STATUS_ICON[attempt.status]
  // Stage vocabulary (Build/Roll out) reuses computeDeployStages, the
  // same derivation the live deploy view uses: only meaningful for the
  // latest attempt, since reconcile conditions carry no per-attempt
  // history (see that function's own doc comment). Failed shows which
  // stage failed; running shows which stage is currently in flight.
  const stageLabel = isLatestAttempt
    ? computeDeployStages(attempt, conditions, true).find((s) =>
        attempt.status === 'failed'
          ? s.status === 'failed'
          : s.status === 'running',
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
      <Checkbox
        className="mt-1"
        checked={isSelected}
        onCheckedChange={onToggleSelected}
        aria-label={`Select ${attempt.image} to compare`}
      />
      <Badge
        variant={DEPLOY_ATTEMPT_STATUS_BADGE_VARIANT[attempt.status]}
        className="mt-0.5 shrink-0 rounded-full"
      >
        <StatusIcon
          className={
            attempt.status === 'running' ? 'size-3 animate-spin' : 'size-3'
          }
        />
        {DEPLOY_ATTEMPT_STATUS_LABEL[attempt.status]}
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
              {DEPLOY_ATTEMPT_SOURCE_LABEL[attempt.source]}
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
        <Button
          variant="outline"
          size="sm"
          nativeButton={false}
          render={
            <Link
              to="/apps/$name/deploys/compare"
              params={{ name: appName }}
              search={{ from: attempt.id }}
            />
          }
        >
          <GitDiffIcon className="size-3.5" data-icon="inline-start" />
          Compare to current
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
