import { useTranslation } from 'react-i18next'
import {
  ArrowUpGlassIcon,
  EllipsisGlassIcon,
  ExternalLinkGlassIcon,
  PowerGlassIcon,
  RotateCcwGlassIcon,
  RotateCwGlassIcon,
  ScrollTextGlassIcon,
  SquareGlassIcon,
} from '@thesvg/ui-react'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import type { ActionState } from '@/lib/upgradeControl'

export interface UpdateActionBarProps {
  actions: ActionState
  targetVersion: string
  checking: boolean
  busy: boolean
  releaseUrl: string | null
  onCheck: () => void
  onUpdate: () => void
  onCancel: () => void
  onRetry: () => void
  onRestart: () => void
  onViewLogs: () => void
}

// Sticky so the right action stays reachable while the page scrolls.
export function UpdateActionBar({
  actions,
  targetVersion,
  checking,
  busy,
  releaseUrl,
  onCheck,
  onUpdate,
  onCancel,
  onRetry,
  onRestart,
  onViewLogs,
}: UpdateActionBarProps) {
  const { t } = useTranslation('updates')
  const variant = (a: ActionState['primary']) =>
    actions.primary === a ? 'default' : 'outline'
  return (
    <div
      role="toolbar"
      aria-label={t('control.barLabel')}
      className="sticky top-0 z-20 -mx-1 flex flex-wrap items-center gap-2 rounded-lg border border-border bg-card/95 px-2 py-2 backdrop-blur"
    >
      {actions.showUpdate ? (
        <Button
          type="button"
          size="sm"
          variant={variant('update')}
          disabled={busy}
          onClick={onUpdate}
        >
          <ArrowUpGlassIcon size="1.4em" />
          {t('control.update', { version: targetVersion })}
        </Button>
      ) : null}
      {actions.showRetry ? (
        <Button
          type="button"
          size="sm"
          variant={variant('retry')}
          disabled={busy}
          onClick={onRetry}
        >
          <RotateCcwGlassIcon size="1.4em" />
          {t('control.retry')}
        </Button>
      ) : null}
      {actions.showCancel ? (
        <Button
          type="button"
          size="sm"
          variant={variant('cancel')}
          disabled={busy || actions.cancelling}
          onClick={onCancel}
        >
          <SquareGlassIcon size="1.4em" />
          {actions.cancelling ? t('control.cancelling') : t('control.cancel')}
        </Button>
      ) : null}
      {actions.showCheck ? (
        <Button
          type="button"
          size="sm"
          variant={variant('check')}
          disabled={checking}
          onClick={onCheck}
        >
          <RotateCwGlassIcon
            size="1.4em"
            className={checking ? 'motion-safe:animate-spin' : ''}
          />
          {checking ? t('status.checking') : t('status.checkNow')}
        </Button>
      ) : null}
      <div className="ml-auto flex items-center gap-2">
        <Button
          type="button"
          size="sm"
          variant="ghost"
          className="hidden sm:inline-flex"
          onClick={onViewLogs}
        >
          <ScrollTextGlassIcon size="1.4em" />
          {t('control.viewLogs')}
        </Button>
        {releaseUrl ? (
          <a
            href={releaseUrl}
            target="_blank"
            rel="noopener noreferrer"
            className={cn(
              buttonVariants({ variant: 'ghost', size: 'sm' }),
              'hidden sm:inline-flex',
            )}
          >
            {t('status.viewNotes')}
            <ExternalLinkGlassIcon size="1.4em" />
          </a>
        ) : null}
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <Button
                type="button"
                variant="outline"
                size="icon-sm"
                aria-label={t('control.more')}
              />
            }
          >
            <EllipsisGlassIcon size="1.4em" />
          </DropdownMenuTrigger>
          <DropdownMenuContent aria-label={t('control.more')}>
            <DropdownMenuItem onClick={onViewLogs} className="sm:hidden">
              <ScrollTextGlassIcon size="1.4em" aria-hidden="true" />
              {t('control.viewLogs')}
            </DropdownMenuItem>
            {releaseUrl ? (
              <DropdownMenuItem
                className="sm:hidden"
                onClick={() =>
                  window.open(releaseUrl, '_blank', 'noopener,noreferrer')
                }
              >
                <ExternalLinkGlassIcon size="1.4em" aria-hidden="true" />
                {t('status.viewNotes')}
              </DropdownMenuItem>
            ) : null}
            <DropdownMenuItem onClick={onRestart}>
              <PowerGlassIcon size="1.4em" aria-hidden="true" />
              {t('control.restartServer')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  )
}
