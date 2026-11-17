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
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
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

// One primary action at a time; the rest live in the More menu.
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
      role="group"
      aria-label={t('control.barLabel')}
      className="flex shrink-0 flex-wrap items-center gap-2"
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
      {actions.showCheck &&
      (actions.primary === 'check' || actions.showRetry) ? (
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
      <div className="flex items-center gap-2">
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
            <DropdownMenuItem onClick={onViewLogs}>
              <ScrollTextGlassIcon size="1.4em" aria-hidden="true" />
              {t('control.viewLogs')}
            </DropdownMenuItem>
            {releaseUrl ? (
              <DropdownMenuItem
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
