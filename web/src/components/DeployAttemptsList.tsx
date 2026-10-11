import { Link, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ScrollIcon,
  ArrowCounterClockwiseIcon,
  DotsThreeIcon,
  ProhibitIcon,
  RocketLaunchIcon,
  GitBranchIcon,
  GitCommitIcon,
  GitDiffIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { DeployAttempt } from '../types/deployAttempt'
import type { ReconcileCondition } from '../types/deploy'
import type { EnvironmentResource } from '../types/environment'
import { useApp } from '../queries/apps'
import { isPendingApproval, useTriggerDeploy } from '../queries/deploys'
import { useRollbackToDeploy } from '../queries/deployControl'
import { useProtectedEnvironment } from '../queries/environments'
import { formatDeployDuration } from '../lib/deployDuration'
import { computeDeployStages } from '../lib/deployStages'
import {
  DEPLOY_ATTEMPT_SOURCE_LABEL,
  DEPLOY_ATTEMPT_STATUS_ICON,
  DEPLOY_ATTEMPT_STATUS_LABEL,
  DEPLOY_ATTEMPT_STATUS_TONE,
} from '../lib/deployAttemptPresentation'
import { ProtectedEnvironmentNotice } from './ProtectedEnvironmentNotice'
import { CancelDeployDialog } from './CancelDeployDialog'
import { ActionMenu, InfoTip, RelativeTime } from './kit'
import { RolloutChip } from './DeployDigestChips'
import { cn } from '@/lib/utils'
import { commitSubject } from '../lib/deploymentPresentation'
import { unpinnedImage } from '../lib/imageDigest'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { EmptyState } from '@/components/ui/empty-state'
import { toast, toastAction } from '@/components/ui/toast'

const TONE_TEXT: Record<string, string> = {
  success: 'text-tone-success',
  danger: 'text-tone-danger',
  warning: 'text-tone-warning',
  info: 'text-tone-info',
  neutral: 'text-muted-foreground',
}

function shortTag(image: string): string {
  const tag = unpinnedImage(image).split(':').pop() ?? image
  return /^[0-9a-f]{40}$/.test(tag) ? tag.slice(0, 7) : tag
}

function durationSuffix(attempt: DeployAttempt): string {
  if (attempt.finished_at) {
    const ms =
      new Date(attempt.finished_at).getTime() -
      new Date(attempt.started_at).getTime()
    if (ms < 1000) return ''
  }
  return ` · ${formatDeployDuration(attempt.started_at, attempt.finished_at)}`
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
  onDeploy,
}: {
  appName: string
  attempts: DeployAttempt[]
  conditions?: ReconcileCondition[]
  onDeploy?: () => void
}) {
  // Capped at 2: a comparison is always between exactly two points (or
  // one point and "current", the per-row Compare-to-current button
  // below), so a third pick replaces the oldest selection rather than
  // growing an N-way comparison this feature doesn't support. Declared
  // before the empty-state early return below so hook order stays fixed
  // regardless of attempts.length.
  const [selected, setSelected] = useState<string[]>([])
  const [ackProtected, setAckProtected] = useState(false)
  const { data: app } = useApp(appName)
  const protectedEnv = useProtectedEnvironment(app)

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
            description="Deploy an existing image tag, or build one from a git source."
            action={
              onDeploy ? (
                <Button size="sm" onClick={onDeploy}>
                  <RocketLaunchIcon
                    className="size-3.5"
                    data-icon="inline-start"
                  />
                  Deploy
                </Button>
              ) : undefined
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
      .sort(
        (x, y) =>
          new Date(x.started_at).getTime() - new Date(y.started_at).getTime(),
      )
    if (!a || !b) return null
    return { from: a.id, to: b.id }
  })()

  return (
    <Card>
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2 space-y-0">
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
        {protectedEnv?.protected ? (
          <ProtectedEnvironmentNotice
            id="deploy-attempts-ack-protected"
            environmentName={protectedEnv.name}
            acknowledged={ackProtected}
            onAcknowledgedChange={setAckProtected}
          />
        ) : null}
        <ul className="-mx-4 -mb-4 divide-y divide-border border-t border-border">
          {attempts.map((a) => (
            <DeployAttemptRow
              key={a.id}
              appName={appName}
              attempt={a}
              isLatestAttempt={a.id === latestAttemptId}
              conditions={conditions}
              isSelected={selected.includes(a.id)}
              onToggleSelected={() => toggleSelected(a.id)}
              protectedEnv={protectedEnv}
              ackProtected={ackProtected}
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
  protectedEnv,
  ackProtected,
}: {
  appName: string
  attempt: DeployAttempt
  isLatestAttempt: boolean
  conditions: ReconcileCondition[]
  isSelected: boolean
  onToggleSelected: () => void
  protectedEnv: EnvironmentResource | undefined
  ackProtected: boolean
}) {
  const { t } = useTranslation('common')
  const navigate = useNavigate()
  const triggerDeploy = useTriggerDeploy(appName)
  const rollbackTo = useRollbackToDeploy(appName)
  const [cancelDialogOpen, setCancelDialogOpen] = useState(false)
  const isCancelable =
    attempt.status === 'queued' ||
    attempt.status === 'running' ||
    attempt.status === 'held'
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

  const rollbackBlocked =
    triggerDeploy.isPending ||
    rollbackTo.isPending ||
    (protectedEnv?.protected && !ackProtected)

  const notifyRollback = (result: Parameters<typeof isPendingApproval>[0]) => {
    const pending = isPendingApproval(result)
    toast.add({
      title: pending
        ? 'Rollback is waiting for approval.'
        : 'Rollback triggered.',
      description: `Redeploying ${unpinnedImage(attempt.image)}.`,
      type: 'success',
      actionProps: pending
        ? undefined
        : toastAction(t('actions.viewOverview'), () => {
            void navigate({
              to: '/apps/$name/overview',
              params: { name: appName },
            })
          }),
    })
  }

  const handleRollback = () => {
    if (attempt.image_digest) {
      rollbackTo.mutate(
        { deployId: attempt.id, confirm: ackProtected },
        {
          onSuccess: notifyRollback,
          onError: (error) =>
            toast.add({
              title: 'Could not roll back.',
              description: error.message,
              type: 'error',
            }),
        },
      )
      return
    }
    triggerDeploy.mutate(
      { image: attempt.image, confirm: ackProtected },
      { onSuccess: notifyRollback },
    )
  }

  const menuItems = [
    {
      id: 'compare',
      label: 'Compare to current',
      description: 'See what changed against the live release',
      icon: <GitDiffIcon className="size-4" />,
      onSelect: () => {
        void navigate({
          to: '/apps/$name/deploys/compare',
          params: { name: appName },
          search: { from: attempt.id },
        })
      },
    },
    ...(attempt.status === 'succeeded'
      ? [
          {
            id: 'rollback',
            label: 'Roll back to this',
            description: attempt.image_digest
              ? 'Redeploys exactly this image, pinned by digest'
              : 'No digest recorded: redeploys by image tag',
            icon: <ArrowCounterClockwiseIcon className="size-4" />,
            disabled: rollbackBlocked,
            onSelect: handleRollback,
          },
        ]
      : []),
    ...(isCancelable
      ? [
          {
            id: 'cancel',
            label: 'Cancel deploy',
            description:
              attempt.status === 'running'
                ? 'Stops the build; the serving release is untouched'
                : 'Removes it from the queue',
            icon: <ProhibitIcon className="size-4" />,
            tone: 'danger' as const,
            onSelect: () => {
              setCancelDialogOpen(true)
            },
          },
        ]
      : []),
  ]

  const title =
    (attempt.commit_message && commitSubject(attempt.commit_message)) ||
    shortTag(attempt.image)
  const StatusIcon = DEPLOY_ATTEMPT_STATUS_ICON[attempt.status]
  const tone = DEPLOY_ATTEMPT_STATUS_TONE[attempt.status]
  const live = attempt.rollout_state === 'serving'
  const detail = {
    to: '/apps/$name/deploys/$deployId/logs' as const,
    params: { name: appName, deployId: attempt.id },
  }
  const note =
    attempt.status === 'queued'
      ? `Position ${attempt.queue_position ?? 1}${attempt.wait_reason ? `, ${attempt.wait_reason}` : ''}`
      : attempt.status === 'held'
        ? attempt.wait_reason
        : attempt.status === 'superseded' && attempt.superseded_by
          ? `Superseded by ${attempt.superseded_by}`
          : attempt.status === 'canceled'
            ? `Canceled${attempt.canceled_by ? ` by ${attempt.canceled_by}` : ''}`
            : attempt.reason

  return (
    <li
      className={cn(
        'group relative flex items-start gap-3 px-4 py-3.5 transition-colors hover:bg-muted/40',
        isSelected && 'bg-muted/50',
      )}
    >
      <div className="relative mt-0.5 size-5 shrink-0 pointer-coarse:flex pointer-coarse:size-auto pointer-coarse:items-center pointer-coarse:gap-2">
        <span
          className={cn(
            'absolute inset-0 flex items-center justify-center transition-opacity pointer-coarse:static pointer-coarse:opacity-100',
            isSelected ? 'opacity-0' : 'group-hover:opacity-0',
            TONE_TEXT[tone],
          )}
        >
          <StatusIcon
            weight="fill"
            className={cn(
              'size-5',
              attempt.status === 'running' &&
                'animate-spin motion-reduce:animate-none',
            )}
            aria-label={DEPLOY_ATTEMPT_STATUS_LABEL[attempt.status]}
          />
        </span>
        <Checkbox
          className={cn(
            'absolute inset-0 m-auto transition-opacity pointer-coarse:static pointer-coarse:m-0 pointer-coarse:opacity-100',
            isSelected ? 'opacity-100' : 'opacity-0 group-hover:opacity-100',
          )}
          checked={isSelected}
          onCheckedChange={onToggleSelected}
          aria-label={`Select ${title} to compare`}
        />
      </div>

      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <Link
            {...detail}
            className="max-w-full truncate text-sm font-medium text-foreground after:absolute after:inset-0 hover:underline"
            title={attempt.commit_message || attempt.image}
          >
            {title}
          </Link>
          {live ? (
            <Badge variant="success" className="shrink-0">
              Current
            </Badge>
          ) : null}
          {stageLabel ? (
            <Badge variant="muted" className="shrink-0">
              {stageLabel}
            </Badge>
          ) : null}
        </div>

        <p className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
          {attempt.branch ? (
            <span className="inline-flex items-center gap-1">
              <GitBranchIcon className="size-3.5" aria-hidden="true" />
              <span className="font-mono">{attempt.branch}</span>
            </span>
          ) : null}
          {attempt.commit_sha ? (
            <span className="inline-flex items-center gap-1 font-mono">
              <GitCommitIcon className="size-3.5" aria-hidden="true" />
              {attempt.commit_sha.slice(0, 7)}
            </span>
          ) : null}
          {attempt.source ? (
            <span>{DEPLOY_ATTEMPT_SOURCE_LABEL[attempt.source]}</span>
          ) : null}
          {attempt.author ? <span>{attempt.author}</span> : null}
          <span title={new Date(attempt.started_at).toLocaleString()}>
            <RelativeTime at={attempt.started_at} live />
            {durationSuffix(attempt)}
          </span>
          {attempt.rollout_state === 'mismatch' ? (
            <RolloutChip attempt={attempt} />
          ) : null}
        </p>

        {note ? (
          <p className="mt-1.5 flex items-center gap-1 text-xs text-muted-foreground">
            {note}
            {attempt.status === 'queued' ? (
              <InfoTip label="About the queue">
                Deploys of one app run one at a time. This one starts on its own
                when the deploy ahead of it finishes or is canceled.
              </InfoTip>
            ) : null}
          </p>
        ) : null}

        {attempt.failure || attempt.error ? (
          <div className="mt-2 flex items-start gap-2 rounded-md border border-tone-danger-border bg-tone-danger-soft px-3 py-2 text-sm">
            <WarningCircleIcon
              weight="fill"
              className="mt-0.5 size-4 shrink-0 text-tone-danger"
              aria-hidden="true"
            />
            <div className="min-w-0">
              <p className="line-clamp-2 font-medium break-words text-tone-danger">
                {attempt.failure?.cause ?? attempt.error}
              </p>
              {attempt.failure?.suggested_fix ? (
                <p className="mt-0.5 line-clamp-2 text-xs break-words text-muted-foreground">
                  {attempt.failure.suggested_fix}
                </p>
              ) : null}
            </div>
          </div>
        ) : null}
      </div>

      <div className="relative z-10 flex shrink-0 items-center gap-1.5">
        <Button
          variant="outline"
          size="sm"
          nativeButton={false}
          className="max-sm:hidden"
          render={<Link {...detail} />}
        >
          <ScrollIcon className="size-3.5" data-icon="inline-start" />
          {attempt.status === 'running' ? 'Watch' : 'Logs'}
        </Button>
        <ActionMenu
          trigger={
            <Button
              variant="ghost"
              size="sm"
              aria-label={`More actions for ${title}`}
            >
              <DotsThreeIcon weight="bold" className="size-4" />
            </Button>
          }
          items={menuItems}
        />
      </div>

      {isCancelable ? (
        <CancelDeployDialog
          appName={appName}
          deployId={attempt.id}
          running={attempt.status === 'running'}
          open={cancelDialogOpen}
          onOpenChange={setCancelDialogOpen}
        />
      ) : null}
    </li>
  )
}
