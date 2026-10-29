import { Link, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  BanGlassIcon,
  CircleAlertGlassIcon,
  CommitGraphGlassIcon,
  DatabaseGlassIcon,
  EllipsisGlassIcon,
  ExternalLinkGlassIcon,
  GitBranchGlassIcon,
  GitCompareGlassIcon,
  LockGlassIcon,
  RefreshCwGlassIcon,
  RotateCcwGlassIcon,
  ScrollGlassIcon,
} from '@thesvg/ui-react'
import type { DeployAttempt } from '../types/deployAttempt'
import type { ReconcileCondition } from '../types/deploy'
import type { EnvironmentResource } from '../types/environment'
import { isPendingApproval, useTriggerDeploy } from '../queries/deploys'
import { useRollbackToDeploy } from '../queries/deployControl'
import { usePublishLockActions } from '../queries/publishLock'
import { formatDeployDuration } from '../lib/deployDuration'
import { computeDeployStages } from '../lib/deployStages'
import {
  DEPLOY_ATTEMPT_SOURCE_LABEL,
  DEPLOY_ATTEMPT_STATUS_ICON,
  DEPLOY_ATTEMPT_STATUS_LABEL,
  DEPLOY_ATTEMPT_STATUS_TONE,
} from '../lib/deployAttemptPresentation'
import { CancelDeployDialog } from './CancelDeployDialog'
import { RollbackWithDataDialog } from './RollbackWithDataDialog'
import { DeployFailureActions } from './DeployFailureActions'
import { DeployFailureDetails } from './DeployFailureDetails'
import { DeployFromCacheButton } from './DeployFromCacheButton'
import { InfoTip, ActionMenu, RelativeTime } from './kit'
import { RolloutChip } from './DeployDigestChips'
import { DeployPinnedStatus } from './DeployPinnedStatus'
import { cn } from '@/lib/utils'
import { commitSubject } from '../lib/deploymentPresentation'
import { unpinnedImage } from '../lib/imageDigest'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { toast, toastAction } from '@/components/ui/toast'

// The reason code a poll-started deploy carries; its label already says why.
const POLLED_REASON = 'DetectedByPoll'

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

export function DeployAttemptRow({
  appName,
  attempt,
  isLatestAttempt,
  conditions,
  isSelected,
  onToggleSelected,
  protectedEnv,
  ackProtected,
  visitUrl,
  pinned = false,
}: {
  appName: string
  visitUrl?: string
  pinned?: boolean
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
  const { publish, rollbackAndLock } = usePublishLockActions(appName)
  const [cancelDialogOpen, setCancelDialogOpen] = useState(false)
  const [dataDialogOpen, setDataDialogOpen] = useState(false)
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
        ? t('deployHistory.promotedPending')
        : t('deployHistory.promoted'),
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
              title: t('deployHistory.promoteFailed'),
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

  // The server marks exactly one deploy live; rollout_state stays 'serving' on
  // superseded rows, so it cannot decide this.
  const live = attempt.live === true
  const held = attempt.publish_state === 'held'
  const lockToastError = (error: Error) =>
    toast.add({
      title: t('publishLock.error', { ns: 'deploySafety' }),
      description: error.message,
      type: 'error',
    })
  const handlePublish = () =>
    publish.mutate(attempt.id, {
      onSuccess: () =>
        toast.add({
          title: t('publishLock.publishedToast', { ns: 'deploySafety' }),
          type: 'success',
        }),
      onError: lockToastError,
    })
  const menuItems = [
    {
      id: 'compare',
      label: 'Compare to current',
      description: 'See what changed against the live release',
      icon: <GitCompareGlassIcon className="size-5" />,
      onSelect: () => {
        void navigate({
          to: '/apps/$name/deploys/compare',
          params: { name: appName },
          search: { from: attempt.id },
        })
      },
    },
    ...(attempt.status === 'succeeded' && !live && !held
      ? [
          {
            id: 'rollback',
            label: t('deployHistory.promote'),
            description: attempt.image_digest
              ? t('deployHistory.promoteHint')
              : 'No digest recorded: redeploys by image tag',
            icon: <RotateCcwGlassIcon className="size-5" />,
            disabled: rollbackBlocked,
            onSelect: handleRollback,
          },
        ]
      : []),
    ...(attempt.status === 'succeeded' && !live && !held
      ? [
          {
            id: 'rollback-lock',
            label: t('publishLock.rollbackLock', { ns: 'deploySafety' }),
            description: t('publishLock.rollbackLockHint', {
              ns: 'deploySafety',
            }),
            icon: <LockGlassIcon className="size-5" />,
            disabled: rollbackBlocked || rollbackAndLock.isPending,
            onSelect: () =>
              rollbackAndLock.mutate(attempt.id, { onError: lockToastError }),
          },
        ]
      : []),
    ...(attempt.status === 'succeeded'
      ? [
          {
            id: 'rollback-with-data',
            label: t('rollbackWithData.menuLabel', { ns: 'deploys' }),
            description: t('rollbackWithData.menuHint', { ns: 'deploys' }),
            icon: <DatabaseGlassIcon className="size-5" />,
            onSelect: () => {
              setDataDialogOpen(true)
            },
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
            icon: <BanGlassIcon className="size-5" />,
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
            : attempt.reason === POLLED_REASON
              ? undefined
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
              {t('deployHistory.current')}
            </Badge>
          ) : null}
          {attempt.trigger === 'rollback' ? (
            <Badge variant="muted" className="shrink-0">
              {t('deployHistory.rollbackBadge')}
            </Badge>
          ) : null}
          {attempt.outcome === 'failed' && attempt.status !== 'failed' ? (
            <Badge
              variant="destructive"
              className="shrink-0"
              title={attempt.outcome_reason}
            >
              {t('deployHistory.outcomeFailed')}
            </Badge>
          ) : null}
          {stageLabel ? (
            <Badge variant="muted" className="shrink-0">
              {stageLabel}
            </Badge>
          ) : null}
          {attempt.rerun_of ? (
            <Link
              to="/apps/$name/deploys/$deployId/logs"
              params={{ name: appName, deployId: attempt.rerun_of }}
              className="relative z-10 inline-flex shrink-0 items-center gap-1 text-xs text-muted-foreground hover:text-foreground hover:underline"
              aria-label={t('deployHistory.rerunOfLabel', {
                id: attempt.rerun_of,
              })}
            >
              <RefreshCwGlassIcon className="size-4.5" aria-hidden="true" />
              {t('deployHistory.rerunOf', {
                id: attempt.rerun_of.slice(0, 12),
              })}
            </Link>
          ) : null}
        </div>

        {pinned ? (
          <DeployPinnedStatus
            appName={appName}
            attempt={attempt}
            stageLabel={stageLabel}
          />
        ) : null}

        <p className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
          {attempt.branch ? (
            <span className="inline-flex items-center gap-1">
              <GitBranchGlassIcon className="size-5" aria-hidden="true" />
              <span className="font-mono">{attempt.branch}</span>
            </span>
          ) : null}
          {attempt.commit_sha ? (
            <span className="inline-flex items-center gap-1 font-mono">
              <CommitGraphGlassIcon className="size-5" aria-hidden="true" />
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
            <CircleAlertGlassIcon
              className="mt-0.5 size-5 shrink-0 text-tone-danger"
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
              <DeployFailureDetails failure={attempt.failure} />
              {attempt.failure ? (
                <DeployFailureActions
                  failure={attempt.failure}
                  appName={appName}
                  image={attempt.image}
                />
              ) : null}
              {attempt.failure?.code === 'git_host_unreachable' ? (
                <DeployFromCacheButton appName={appName} />
              ) : null}
            </div>
          </div>
        ) : null}
      </div>

      <div className="relative z-10 flex shrink-0 items-center gap-1.5">
        {live && visitUrl ? (
          <Button
            variant="outline"
            size="sm"
            nativeButton={false}
            render={
              <a
                href={visitUrl}
                target="_blank"
                rel="noopener noreferrer"
                aria-label={t('deployHistory.visitLabel', { url: visitUrl })}
              />
            }
          >
            <ExternalLinkGlassIcon
              className="size-5"
              data-icon="inline-start"
            />
            {t('deployHistory.visit')}
          </Button>
        ) : null}
        {!live && attempt.deploy_url ? (
          <Button
            variant="outline"
            size="sm"
            nativeButton={false}
            render={
              <a
                href={attempt.deploy_url}
                target="_blank"
                rel="noopener noreferrer"
                aria-label={t('deployHistory.visitLabel', {
                  url: attempt.deploy_url,
                })}
              />
            }
          >
            <ExternalLinkGlassIcon
              className="size-5"
              data-icon="inline-start"
            />
            {t('deployHistory.visit')}
          </Button>
        ) : null}
        {held ? (
          <Button
            size="sm"
            disabled={publish.isPending}
            onClick={handlePublish}
          >
            {t('publishLock.publish', { ns: 'deploySafety' })}
          </Button>
        ) : null}
        {attempt.status === 'succeeded' && !live && !held ? (
          <Button
            variant="outline"
            size="sm"
            className="max-sm:hidden"
            disabled={rollbackBlocked}
            onClick={handleRollback}
          >
            <RotateCcwGlassIcon className="size-5" data-icon="inline-start" />
            {t('deployHistory.promote')}
          </Button>
        ) : null}
        <Button
          variant="outline"
          size="sm"
          nativeButton={false}
          className="max-sm:hidden"
          render={<Link {...detail} />}
        >
          <ScrollGlassIcon className="size-5" data-icon="inline-start" />
          {attempt.status === 'running'
            ? t('deployHistory.watchLog')
            : attempt.status === 'failed'
              ? t('deployHistory.failedLog')
              : t('deployHistory.viewLogs')}
        </Button>
        <ActionMenu
          trigger={
            <Button
              variant="ghost"
              size="sm"
              aria-label={`More actions for ${title}`}
            >
              <EllipsisGlassIcon className="size-5" />
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
      {attempt.status === 'succeeded' && dataDialogOpen ? (
        <RollbackWithDataDialog
          appName={appName}
          deployId={attempt.id}
          protectedEnv={protectedEnv?.protected === true}
          open={dataDialogOpen}
          onOpenChange={setDataDialogOpen}
        />
      ) : null}
    </li>
  )
}
