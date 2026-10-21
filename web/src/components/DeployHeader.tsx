import { Display } from '@/components/ui/typography'
import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowCounterClockwiseIcon,
  ArrowLeftIcon,
  ArrowSquareOutIcon,
  CheckIcon,
  ClockIcon,
  CopyIcon,
  GitBranchIcon,
  GitCommitIcon,
  GitDiffIcon,
  LightningIcon,
  ProhibitIcon,
  UserIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { DeployAttempt } from '../types/deployAttempt'
import {
  DEPLOY_ATTEMPT_SOURCE_LABEL,
  DEPLOY_ATTEMPT_STATUS_LABEL,
  DEPLOY_ATTEMPT_STATUS_TONE,
} from '../lib/deployAttemptPresentation'
import { commitSubject } from '../lib/deploymentPresentation'
import { formatDeployDuration } from '../lib/deployDuration'
import { imageTagOf } from './overview/imageTag'
import { useRollbackToDeploy } from '../queries/deployControl'
import { RelativeTime, StatusPill, type Tone } from './kit'
import { CancelDeployDialog } from './CancelDeployDialog'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'

const SHA_RE = /^[0-9a-f]{40}$/

function Meta({
  icon,
  children,
  title,
}: {
  icon: React.ReactNode
  children: React.ReactNode
  title?: string
}) {
  return (
    <span className="inline-flex items-center gap-1.5" title={title}>
      <span className="text-muted-foreground/70" aria-hidden="true">
        {icon}
      </span>
      {children}
    </span>
  )
}

export function DeployHeader({
  appName,
  attempt,
  domain,
  branch,
  isLive,
  displayStatus,
}: {
  appName: string
  attempt: DeployAttempt
  domain?: string
  branch?: string
  isLive: boolean
  /** Replaces the raw attempt badge with the page's single derived state. */
  displayStatus?: { tone: Tone; label: string; live?: boolean }
}) {
  const { t } = useTranslation('deployDetail')
  const navigate = useNavigate()
  const rollback = useRollbackToDeploy(appName)
  const [cancelOpen, setCancelOpen] = useState(false)
  const [copied, setCopied] = useState(false)
  const cancelable =
    attempt.status === 'running' ||
    attempt.status === 'queued' ||
    attempt.status === 'held'
  const canRollback = attempt.status === 'succeeded' && !isLive
  const tag = imageTagOf(attempt.image)
  const title =
    (attempt.commit_message && commitSubject(attempt.commit_message)) ||
    (SHA_RE.test(tag) ? tag.slice(0, 7) : tag)
  const branchName = attempt.branch || branch
  const running = attempt.status === 'running' || attempt.status === 'queued'

  const copyId = () => {
    void navigator.clipboard.writeText(attempt.id).then(() => {
      setCopied(true)
      window.setTimeout(() => {
        setCopied(false)
      }, 1500)
    })
  }

  const doRollback = () => {
    rollback.mutate(
      { deployId: attempt.id, confirm: false },
      {
        onSuccess: () => {
          toast.add({ title: 'Rollback triggered.', type: 'success' })
          void navigate({
            to: '/apps/$name/overview',
            params: { name: appName },
          })
        },
        onError: (e) =>
          toast.add({
            title: 'Could not roll back.',
            description: e.message,
            type: 'error',
          }),
      },
    )
  }

  return (
    <header className="space-y-3">
      <Link
        to="/apps/$name/deploys"
        params={{ name: appName }}
        className="inline-flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground"
      >
        <ArrowLeftIcon className="size-3.5" aria-hidden="true" />
        All deploys
      </Link>

      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-2">
          <div className="flex flex-wrap items-center gap-2.5">
            <Display as="h1" size="page" className="max-w-full truncate">
              {title}
            </Display>
            <StatusPill
              tone={
                displayStatus?.tone ??
                DEPLOY_ATTEMPT_STATUS_TONE[attempt.status]
              }
              label={
                displayStatus?.label ??
                DEPLOY_ATTEMPT_STATUS_LABEL[attempt.status]
              }
              live={displayStatus?.live ?? running}
            />
            {isLive ? <StatusPill tone="success" label="Current" /> : null}
            {attempt.chain_position && attempt.chain_position > 1 ? (
              <StatusPill
                tone="neutral"
                label={t('chain.position', {
                  position: attempt.chain_position,
                })}
              />
            ) : null}
          </div>
          <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 text-sm text-muted-foreground">
            {branchName ? (
              <Meta icon={<GitBranchIcon className="size-4" />}>
                <span className="break-all font-mono text-foreground">
                  {branchName}
                </span>
              </Meta>
            ) : null}
            {attempt.commit_sha ? (
              <Meta
                icon={<GitCommitIcon className="size-4" />}
                title={attempt.commit_sha}
              >
                <span className="break-all font-mono text-foreground">
                  {attempt.commit_sha.slice(0, 7)}
                </span>
              </Meta>
            ) : null}
            {attempt.author ? (
              <Meta icon={<UserIcon className="size-4" />}>
                {attempt.author}
              </Meta>
            ) : null}
            {attempt.source ? (
              <Meta icon={<LightningIcon className="size-4" />}>
                {DEPLOY_ATTEMPT_SOURCE_LABEL[attempt.source]}
              </Meta>
            ) : null}
            <Meta icon={<ClockIcon className="size-4" />}>
              <RelativeTime at={attempt.started_at} live />
              <span aria-hidden="true">·</span>
              {formatDeployDuration(attempt.started_at, attempt.finished_at)}
            </Meta>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          {cancelable ? (
            <Button
              variant="outline"
              size="sm"
              writeAction
              onClick={() => {
                setCancelOpen(true)
              }}
            >
              <ProhibitIcon className="size-3.5" data-icon="inline-start" />
              Cancel deploy
            </Button>
          ) : null}
          {canRollback ? (
            <Button
              variant="outline"
              size="sm"
              writeAction
              disabled={rollback.isPending}
              onClick={doRollback}
            >
              <ArrowCounterClockwiseIcon
                className="size-3.5"
                data-icon="inline-start"
              />
              Roll back to this
            </Button>
          ) : null}
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
            Compare
          </Button>
          <Button variant="outline" size="sm" onClick={copyId}>
            {copied ? (
              <CheckIcon className="size-3.5" data-icon="inline-start" />
            ) : (
              <CopyIcon className="size-3.5" data-icon="inline-start" />
            )}
            {copied ? 'Copied' : 'Copy ID'}
          </Button>
          {domain && attempt.status === 'succeeded' ? (
            <Button
              size="sm"
              nativeButton={false}
              render={
                <a
                  href={`https://${domain}`}
                  target="_blank"
                  rel="noreferrer"
                />
              }
            >
              <ArrowSquareOutIcon
                className="size-3.5"
                data-icon="inline-start"
              />
              Visit
            </Button>
          ) : null}
        </div>
      </div>

      {cancelable ? (
        <CancelDeployDialog
          appName={appName}
          deployId={attempt.id}
          running={attempt.status === 'running'}
          open={cancelOpen}
          onOpenChange={setCancelOpen}
        />
      ) : null}
    </header>
  )
}
