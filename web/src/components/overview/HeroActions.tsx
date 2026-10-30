import { useState } from 'react'
import {
  ChevronDownGlassIcon,
  CopyGlassIcon,
  EllipsisGlassIcon,
  ExternalLinkGlassIcon,
  GitBranchGlassIcon,
  PackageGlassIcon,
  PauseGlassIcon,
  PlayGlassIcon,
  RocketGlassIcon,
  RotateCwGlassIcon,
  TrashGlassIcon,
  UndoGlassIcon,
} from '@thesvg/ui-react'
import { ActionMenu, Kbd } from '@/components/kit'
import { Button, buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { CloneAppDialog } from '../CloneAppDialog'
import { DeleteAppDialog } from '../DeleteAppDialog'
import { PromoteAppDialog } from '../PromoteAppDialog'
import { SaveAsTemplateDialog } from '../SaveAsTemplateDialog'
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
  const [saveAsTemplateOpen, setSaveAsTemplateOpen] = useState(false)
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
          <ExternalLinkGlassIcon size="1.4em" aria-hidden="true" />
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
          <RocketGlassIcon size="1.4em" aria-hidden="true" />
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
              <ChevronDownGlassIcon size="1.4em" aria-hidden="true" />
            </Button>
          }
          items={[
            {
              id: 'redeploy',
              label: actions.redeploying
                ? 'Redeploying...'
                : 'Redeploy current',
              icon: <RotateCwGlassIcon size="1.4em" />,
              disabled: actions.redeploying,
              onSelect: actions.redeploy,
            },
            {
              id: 'deploy-image',
              label: 'Deploy image...',
              icon: <RocketGlassIcon size="1.4em" />,
              onSelect: () => onDeploy('existing-image'),
            },
            {
              id: 'build-source',
              label: 'Build from source...',
              icon: <GitBranchGlassIcon size="1.4em" />,
              onSelect: () => onDeploy('build-from-source'),
            },
            {
              id: 'rollback',
              label: 'Rollback...',
              icon: <UndoGlassIcon size="1.4em" />,
              onSelect: actions.goToDeploys,
            },
          ]}
        />
      </div>

      <ActionMenu
        trigger={
          <Button variant="ghost" size="icon-lg" aria-label="More actions">
            <EllipsisGlassIcon size="1.4em" aria-hidden="true" />
          </Button>
        }
        items={[
          {
            id: 'restart',
            label: actions.restartLabel,
            icon: <RotateCwGlassIcon size="1.4em" />,
            disabled: actions.restarting,
            onSelect: actions.restart,
          },
          {
            id: 'toggle',
            label: app.suspended
              ? 'Start'
              : actions.stopping
                ? 'Stopping...'
                : 'Stop',
            icon: app.suspended ? (
              <PlayGlassIcon size="1.4em" />
            ) : (
              <PauseGlassIcon size="1.4em" />
            ),
            disabled: actions.togglingRunning,
            onSelect: actions.toggleRunning,
          },
          {
            id: 'copy-url',
            label: 'Copy URL',
            icon: <CopyGlassIcon size="1.4em" />,
            disabled: !url,
            onSelect: actions.copyUrl,
          },
          {
            id: 'promote',
            label: 'Promote...',
            icon: <RocketGlassIcon size="1.4em" />,
            onSelect: () => setPromoteOpen(true),
          },
          {
            id: 'clone',
            label: 'Clone...',
            icon: <CopyGlassIcon size="1.4em" />,
            onSelect: () => setCloneOpen(true),
          },
          {
            id: 'save-as-template',
            label: 'Save as template...',
            icon: <PackageGlassIcon size="1.4em" />,
            onSelect: () => setSaveAsTemplateOpen(true),
          },
          {
            id: 'delete',
            label: 'Delete...',
            icon: <TrashGlassIcon size="1.4em" />,
            tone: 'danger',
            onSelect: () => setDeleteOpen(true),
          },
        ]}
      />

      {actions.confirmDialog}
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
      <SaveAsTemplateDialog
        appName={app.name}
        control={{
          open: saveAsTemplateOpen,
          onOpenChange: setSaveAsTemplateOpen,
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
