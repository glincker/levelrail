import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  DropletsGlassIcon,
  EllipsisGlassIcon,
  OctagonAlertGlassIcon,
  PauseGlassIcon,
  PlayGlassIcon,
  SkipForwardGlassIcon,
  KeyboardGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { RoleGate } from '@/components/ui/RoleGate'
import { useAbilities } from '@/hooks/useAbilities'
import { useMediaQuery } from '@/hooks/useMediaQuery'
import { useBuildQueue } from '../../queries/buildQueue'
import { KillAllDialog } from './KillAllDialog'
import { PauseForm } from './PauseForm'
import { QueueStateChip } from './QueueStateChip'
import { useQueueChange } from './useQueueToasts'

export interface IdleExtras {
  onQueue: () => void
  onShortcuts: () => void
}

// Hub header cluster: one state chip, then actions. While builds run or wait
// they stay direct buttons; when idle (or below sm) they fold into one menu.
export function BuildQueueControls({
  idleExtras,
}: {
  idleExtras?: IdleExtras
}) {
  const { t } = useTranslation('deploys')
  const { data: queue } = useBuildQueue()
  const { can } = useAbilities()
  const wide = useMediaQuery('(min-width: 640px)')
  const change = useQueueChange()
  const [pauseOpen, setPauseOpen] = useState(false)
  const [killOpen, setKillOpen] = useState(false)
  if (!queue) return null

  const running = queue.state === 'running'
  const active = queue.running.length > 0 || queue.queued.length > 0
  const sensitive = can('write:sensitive')
  const folded = !wide || !active
  const submitPause = (reason: string) => {
    change.run('pause', reason, () => {
      setPauseOpen(false)
    })
  }

  const killDialog = (
    <KillAllDialog open={killOpen} onOpenChange={setKillOpen} queue={queue} />
  )
  const menuTrigger = (
    <DropdownMenuTrigger
      render={
        <Button
          type="button"
          size="icon-sm"
          variant="outline"
          aria-label={t('buildQueue.menu')}
        />
      }
    >
      <EllipsisGlassIcon size="1.4em" aria-hidden="true" />
    </DropdownMenuTrigger>
  )

  if (folded) {
    const showExtras = !active && idleExtras
    return (
      <>
        <QueueStateChip queue={queue} />
        <DropdownMenu>
          {menuTrigger}
          <DropdownMenuContent>
            {running ? (
              <DropdownMenuItem
                disabled={!sensitive}
                onClick={() => {
                  setPauseOpen(true)
                }}
              >
                <PauseGlassIcon aria-hidden="true" />
                {t('buildQueue.pause.action')}
              </DropdownMenuItem>
            ) : (
              <DropdownMenuItem
                disabled={!sensitive || change.pending}
                onClick={() => {
                  change.run('resume')
                }}
              >
                <PlayGlassIcon aria-hidden="true" />
                {t('buildQueue.resume')}
              </DropdownMenuItem>
            )}
            <DropdownMenuItem
              disabled={queue.state === 'draining'}
              onClick={() => {
                change.run('drain')
              }}
            >
              <DropletsGlassIcon aria-hidden="true" />
              {t('buildQueue.drain')}
            </DropdownMenuItem>
            {showExtras ? (
              <>
                <DropdownMenuItem onClick={idleExtras.onQueue}>
                  <SkipForwardGlassIcon aria-hidden="true" />
                  {t('supersededPopover.queue')}
                </DropdownMenuItem>
                <DropdownMenuItem onClick={idleExtras.onShortcuts}>
                  <KeyboardGlassIcon aria-hidden="true" />
                  {t('deployList.shortcuts')}
                </DropdownMenuItem>
              </>
            ) : null}
            {active ? (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  className="text-tone-danger [&_svg]:text-tone-danger"
                  disabled={!sensitive}
                  onClick={() => {
                    setKillOpen(true)
                  }}
                >
                  <OctagonAlertGlassIcon aria-hidden="true" />
                  {t('buildQueue.kill.action')}
                </DropdownMenuItem>
              </>
            ) : null}
          </DropdownMenuContent>
        </DropdownMenu>
        <Dialog open={pauseOpen} onOpenChange={setPauseOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t('buildQueue.pause.title')}</DialogTitle>
              <DialogDescription>
                {t('buildQueue.pause.description')}
              </DialogDescription>
            </DialogHeader>
            <PauseForm pending={change.pending} onSubmit={submitPause} />
          </DialogContent>
        </Dialog>
        {killDialog}
      </>
    )
  }

  return (
    <>
      <QueueStateChip queue={queue} />
      {running ? (
        <Popover open={pauseOpen} onOpenChange={setPauseOpen}>
          <RoleGate ability="write:sensitive">
            <PopoverTrigger
              render={<Button type="button" size="sm" variant="outline" />}
            >
              <PauseGlassIcon size="1.4em" aria-hidden="true" />
              {t('buildQueue.pause.action')}
            </PopoverTrigger>
          </RoleGate>
          <PopoverContent align="end" className="w-72">
            <PopoverHeader>
              <PopoverTitle>{t('buildQueue.pause.title')}</PopoverTitle>
              <PopoverDescription>
                {t('buildQueue.pause.description')}
              </PopoverDescription>
            </PopoverHeader>
            <PauseForm pending={change.pending} onSubmit={submitPause} />
          </PopoverContent>
        </Popover>
      ) : (
        <RoleGate ability="write:sensitive">
          <Button
            type="button"
            size="sm"
            variant="outline"
            writeAction
            disabled={change.pending}
            onClick={() => {
              change.run('resume')
            }}
          >
            <PlayGlassIcon size="1.4em" aria-hidden="true" />
            {t('buildQueue.resume')}
          </Button>
        </RoleGate>
      )}
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              type="button"
              size="icon-sm"
              variant="outline"
              aria-label={t('buildQueue.menu')}
            />
          }
        >
          <EllipsisGlassIcon size="1.4em" aria-hidden="true" />
        </DropdownMenuTrigger>
        <DropdownMenuContent>
          <DropdownMenuItem
            disabled={queue.state === 'draining'}
            onClick={() => {
              change.run('drain')
            }}
          >
            <DropletsGlassIcon aria-hidden="true" />
            <span className="flex flex-col">
              <span>{t('buildQueue.drain')}</span>
              <span className="text-xs text-muted-foreground">
                {t('buildQueue.drainHint')}
              </span>
            </span>
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <RoleGate ability="write:sensitive">
        <Button
          type="button"
          size="sm"
          variant="destructive"
          writeAction
          onClick={() => {
            setKillOpen(true)
          }}
        >
          <OctagonAlertGlassIcon size="1.4em" aria-hidden="true" />
          {t('buildQueue.kill.action')}
        </Button>
      </RoleGate>
      {killDialog}
    </>
  )
}
