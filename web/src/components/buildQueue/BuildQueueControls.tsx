import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  DropletsGlassIcon,
  EllipsisGlassIcon,
  OctagonAlertGlassIcon,
  PauseGlassIcon,
  PlayGlassIcon,
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
import { useMediaQuery } from '@/hooks/useMediaQuery'
import { useBuildQueue } from '../../queries/buildQueue'
import { KillAllDialog } from './KillAllDialog'
import { PauseForm } from './PauseForm'
import { QueueStateChip } from './QueueStateChip'
import { useQueueChange } from './useQueueToasts'

// Hub header cluster: state chip, pause or resume, drain, kill all. Below sm
// everything but the chip folds into one overflow menu.
export function BuildQueueControls() {
  const { t } = useTranslation('deploys')
  const { data: queue } = useBuildQueue()
  const wide = useMediaQuery('(min-width: 640px)')
  const change = useQueueChange()
  const [pauseOpen, setPauseOpen] = useState(false)
  const [killOpen, setKillOpen] = useState(false)
  if (!queue) return null

  const running = queue.state === 'running'
  const submitPause = (reason: string) => {
    change.run('pause', reason, () => {
      setPauseOpen(false)
    })
  }

  const killDialog = (
    <KillAllDialog open={killOpen} onOpenChange={setKillOpen} queue={queue} />
  )

  if (!wide) {
    return (
      <>
        <QueueStateChip queue={queue} />
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
            {running ? (
              <DropdownMenuItem
                onClick={() => {
                  setPauseOpen(true)
                }}
              >
                <PauseGlassIcon aria-hidden="true" />
                {t('buildQueue.pause.action')}
              </DropdownMenuItem>
            ) : (
              <DropdownMenuItem
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
            <DropdownMenuSeparator />
            <DropdownMenuItem
              className="text-tone-danger [&_svg]:text-tone-danger"
              onClick={() => {
                setKillOpen(true)
              }}
            >
              <OctagonAlertGlassIcon aria-hidden="true" />
              {t('buildQueue.kill.action')}
            </DropdownMenuItem>
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
