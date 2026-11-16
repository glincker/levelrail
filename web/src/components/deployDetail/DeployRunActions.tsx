import {
  BanGlassIcon,
  CheckGlassIcon,
  DownloadGlassIcon,
  LinkGlassIcon,
  RefreshCwGlassIcon,
  RotateCcwGlassIcon,
} from '@thesvg/ui-react'
import { useNavigate } from '@tanstack/react-router'
import { useEffect, useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { toast } from '@/components/ui/toast'
import { CancelDeployDialog } from '../CancelDeployDialog'
import type { DeployAttempt } from '../../types/deployAttempt'
import { deployLogDownloadURL } from '../../queries/deployLogs'
import { useRollbackToDeploy } from '../../queries/deployControl'
import { useTriggerDeploy } from '../../queries/deploys'
import { useDeployDetailT } from './useDeployDetailT'

const CANCELABLE = new Set(['running', 'queued', 'held', 'awaiting_approval'])

function isTypingTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return (
    target.isContentEditable ||
    ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName)
  )
}

function Tip({
  reason,
  shortcut,
  children,
}: {
  reason?: string
  shortcut: string
  children: ReactNode
}) {
  return (
    <Tooltip>
      <TooltipTrigger render={<span className="inline-flex" />}>
        {children}
      </TooltipTrigger>
      <TooltipContent>
        {reason ? `${reason} ` : ''}
        {shortcut ? <kbd className="font-mono">{shortcut}</kbd> : null}
      </TooltipContent>
    </Tooltip>
  )
}

/** Top-right actions of the run page: stop, retry, roll back, share, download. */
export function DeployRunActions({
  appName,
  attempt,
  isLive,
}: {
  appName: string
  attempt: DeployAttempt
  isLive: boolean
}) {
  const { t } = useDeployDetailT()
  const navigate = useNavigate()
  const rollback = useRollbackToDeploy(appName)
  const redeploy = useTriggerDeploy(appName)
  const [cancelOpen, setCancelOpen] = useState(false)
  const [linkCopied, setLinkCopied] = useState(false)
  const cancelable = CANCELABLE.has(attempt.status)
  const finished = !cancelable
  const canRollback = attempt.status === 'succeeded' && !isLive

  const doRedeploy = () => {
    redeploy.mutate(
      { image: attempt.image, rerunOf: attempt.id },
      {
        onSuccess: () => {
          toast.add({ title: t('runActions.redeployStarted'), type: 'success' })
        },
        onError: (e) =>
          toast.add({
            title: t('runActions.redeployFailed'),
            description: e.message,
            type: 'error',
          }),
      },
    )
  }
  const doRollback = () => {
    rollback.mutate(
      { deployId: attempt.id, confirm: false },
      {
        onSuccess: () => {
          toast.add({ title: t('runActions.rollbackStarted'), type: 'success' })
          void navigate({
            to: '/apps/$name/overview',
            params: { name: appName },
          })
        },
        onError: (e) =>
          toast.add({
            title: t('runActions.rollbackFailed'),
            description: e.message,
            type: 'error',
          }),
      },
    )
  }
  const copyLink = () => {
    const url = `${window.location.origin}${window.location.pathname}#logs`
    void navigator.clipboard.writeText(url).then(() => {
      setLinkCopied(true)
      window.setTimeout(() => {
        setLinkCopied(false)
      }, 1500)
    })
  }

  // Single-modifier shortcuts so they never collide with typing in the log search.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (!e.shiftKey || e.ctrlKey || e.metaKey || e.altKey) return
      if (isTypingTarget(e.target) || cancelOpen) return
      if (e.key === 'X' && cancelable) setCancelOpen(true)
      if (e.key === 'R' && finished) doRedeploy()
      if (e.key === 'L') copyLink()
    }
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('keydown', onKey)
    }
  })

  return (
    <>
      <Tip
        shortcut="Shift+X"
        reason={
          cancelable ? t('runActions.cancelWhy') : t('runActions.cancelOff')
        }
      >
        <Button
          variant="destructive"
          size="sm"
          writeAction
          disabled={!cancelable}
          onClick={() => {
            setCancelOpen(true)
          }}
        >
          <BanGlassIcon className="size-5" data-icon="inline-start" />
          {t('runActions.cancel')}
        </Button>
      </Tip>
      <Tip
        shortcut="Shift+R"
        reason={
          finished ? t('runActions.redeployWhy') : t('runActions.redeployOff')
        }
      >
        <Button
          variant="outline"
          size="sm"
          writeAction
          disabled={!finished || redeploy.isPending}
          onClick={doRedeploy}
        >
          <RefreshCwGlassIcon className="size-5" data-icon="inline-start" />
          {t(
            attempt.status === 'failed'
              ? 'runActions.retry'
              : 'runActions.redeploy',
          )}
        </Button>
      </Tip>
      <Tip
        shortcut=""
        reason={
          canRollback
            ? t('runActions.rollbackWhy')
            : t('runActions.rollbackOff')
        }
      >
        <Button
          variant="outline"
          size="sm"
          writeAction
          disabled={!canRollback || rollback.isPending}
          onClick={doRollback}
        >
          <RotateCcwGlassIcon className="size-5" data-icon="inline-start" />
          {t('runActions.rollback')}
        </Button>
      </Tip>
      <Tip shortcut="Shift+L" reason={t('runActions.copyLinkWhy')}>
        <Button variant="outline" size="sm" onClick={copyLink}>
          {linkCopied ? (
            <CheckGlassIcon className="size-5" data-icon="inline-start" />
          ) : (
            <LinkGlassIcon className="size-5" data-icon="inline-start" />
          )}
          {t(linkCopied ? 'runActions.copied' : 'runActions.copyLink')}
        </Button>
      </Tip>
      <Button
        variant="outline"
        size="sm"
        nativeButton={false}
        render={<a href={deployLogDownloadURL(appName, attempt.id)} download />}
      >
        <DownloadGlassIcon className="size-5" data-icon="inline-start" />
        {t('runActions.download')}
      </Button>
      {cancelable ? (
        <CancelDeployDialog
          appName={appName}
          deployId={attempt.id}
          running={attempt.status === 'running'}
          open={cancelOpen}
          onOpenChange={setCancelOpen}
        />
      ) : null}
    </>
  )
}
