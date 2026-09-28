import { useState } from 'react'
import {
  ArrowArcLeftIcon,
  ArrowClockwiseIcon,
  ArrowSquareOutIcon,
  CaretDownIcon,
  CopyIcon,
  DotsThreeIcon,
  GitBranchIcon,
  PauseIcon,
  PlayIcon,
  RocketLaunchIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { ActionMenu, Kbd } from '@/components/kit'
import { Button, buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { CloneAppDialog } from '../CloneAppDialog'
import { DeleteAppDialog } from '../DeleteAppDialog'
import { PromoteAppDialog } from '../PromoteAppDialog'
import type { AppDetail } from '../../types/appDetail'
import type { DeployTab } from './DeploySheet'
import { useOverviewActions } from './useOverviewActions'

export function HeroActions({
  app,
  url,
  onDeploy,
}: {
  app: AppDetail
  url: string | null
  onDeploy: (tab: DeployTab) => void
}) {
  const actions = useOverviewActions(app, url)
  const [promoteOpen, setPromoteOpen] = useState(false)
  const [cloneOpen, setCloneOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)

  return (
    <div className="flex flex-wrap items-center gap-2">
      {url ? (
        <a
          href={url}
          target="_blank"
          rel="noreferrer"
          className={cn(buttonVariants({ size: 'lg' }), 'gap-2')}
        >
          <ArrowSquareOutIcon aria-hidden="true" />
          Open app
          <Kbd keys={['O']} />
        </a>
      ) : null}

      <div className="flex items-center">
        <Button
          variant="outline"
          size="lg"
          className="rounded-r-none"
          onClick={() => onDeploy('existing-image')}
        >
          <RocketLaunchIcon aria-hidden="true" />
          Deploy
        </Button>
        <ActionMenu
          trigger={
            <Button
              variant="outline"
              size="icon-lg"
              className="-ml-px rounded-l-none"
              aria-label="More deploy options"
            >
              <CaretDownIcon aria-hidden="true" />
            </Button>
          }
          items={[
            {
              id: 'redeploy',
              label: actions.redeploying
                ? 'Redeploying...'
                : 'Redeploy current',
              icon: <ArrowClockwiseIcon />,
              disabled: actions.redeploying,
              onSelect: actions.redeploy,
            },
            {
              id: 'deploy-image',
              label: 'Deploy image...',
              icon: <RocketLaunchIcon />,
              onSelect: () => onDeploy('existing-image'),
            },
            {
              id: 'build-source',
              label: 'Build from source...',
              icon: <GitBranchIcon />,
              onSelect: () => onDeploy('build-from-source'),
            },
            {
              id: 'rollback',
              label: 'Rollback...',
              icon: <ArrowArcLeftIcon />,
              onSelect: actions.goToDeploys,
            },
          ]}
        />
      </div>

      <ActionMenu
        trigger={
          <Button variant="ghost" size="icon-lg" aria-label="More actions">
            <DotsThreeIcon weight="bold" aria-hidden="true" />
          </Button>
        }
        items={[
          {
            id: 'restart',
            label: actions.restarting ? 'Restarting...' : 'Restart',
            icon: <ArrowClockwiseIcon />,
            disabled: actions.restarting,
            onSelect: actions.restart,
          },
          {
            id: 'toggle',
            label: app.suspended ? 'Start' : 'Stop',
            icon: app.suspended ? <PlayIcon /> : <PauseIcon />,
            disabled: actions.togglingRunning,
            onSelect: actions.toggleRunning,
          },
          {
            id: 'copy-url',
            label: 'Copy URL',
            icon: <CopyIcon />,
            disabled: !url,
            onSelect: actions.copyUrl,
          },
          {
            id: 'promote',
            label: 'Promote...',
            icon: <RocketLaunchIcon />,
            onSelect: () => setPromoteOpen(true),
          },
          {
            id: 'clone',
            label: 'Clone...',
            icon: <CopyIcon />,
            onSelect: () => setCloneOpen(true),
          },
          {
            id: 'delete',
            label: 'Delete...',
            icon: <TrashIcon />,
            tone: 'danger',
            onSelect: () => setDeleteOpen(true),
          },
        ]}
      />

      <PromoteAppDialog
        appName={app.name}
        projectId={app.project_id}
        control={{
          open: promoteOpen,
          onOpenChange: setPromoteOpen,
          hideTrigger: true,
        }}
      />
      <CloneAppDialog
        name={app.name}
        control={{
          open: cloneOpen,
          onOpenChange: setCloneOpen,
          hideTrigger: true,
        }}
      />
      <DeleteAppDialog
        name={app.name}
        control={{
          open: deleteOpen,
          onOpenChange: setDeleteOpen,
          hideTrigger: true,
        }}
      />
    </div>
  )
}
