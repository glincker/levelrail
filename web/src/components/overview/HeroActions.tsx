import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  TagGlassIcon,
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
import { AddTagDialog } from '../AddTagDialog'
import { CloneAppDialog } from '../CloneAppDialog'
import { DeleteAppDialog } from '../DeleteAppDialog'
import { PromoteAppDialog } from '../PromoteAppDialog'
import { SaveAsTemplateDialog } from '../SaveAsTemplateDialog'
import type { AppDetail } from '../../types/appDetail'
import type { DeployTab } from './DeploySheet'
import { useOverviewActions } from './useOverviewActions'
import type { HeroPrimary } from './heroPrimary'

export function HeroActions({
  app,
  url,
  onDeploy,
  primary = 'deploy',
  gitMissing = false,
}: {
  app: AppDetail
  url: string | null
  onDeploy: (tab: DeployTab) => void
  primary?: HeroPrimary
  gitMissing?: boolean
}) {
  const actions = useOverviewActions(app, url)
  const [promoteOpen, setPromoteOpen] = useState(false)
  const [cloneOpen, setCloneOpen] = useState(false)
  const [saveAsTemplateOpen, setSaveAsTemplateOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [tagOpen, setTagOpen] = useState(false)
  const { t } = useTranslation('common')

  return (
    <div className="flex shrink-0 flex-wrap items-center gap-2">
      {gitMissing ? (
        <Link
          to="/apps/$name/source"
          params={{ name: app.name }}
          className={buttonVariants({ variant: 'outline', size: 'lg' })}
        >
          <GitBranchGlassIcon size="1.4em" aria-hidden="true" />
          {t('overview.connectGitSource')}
        </Link>
      ) : null}
      {url ? (
        <a
          href={url}
          target="_blank"
          rel="noreferrer"
          className={cn(
            buttonVariants({ variant: 'outline', size: 'lg' }),
            'gap-2',
          )}
        >
          <ExternalLinkGlassIcon size="1.4em" aria-hidden="true" />
          <span className="max-sm:sr-only">{t('overviewHero.openApp')}</span>
          <span className="max-lg:hidden">
            <Kbd keys={['O']} />
          </span>
        </a>
      ) : null}

      <div className="flex items-center">
        <Button
          size="lg"
          className="rounded-r-none"
          disabled={
            (primary === 'restart' && actions.restarting) ||
            (primary === 'retry' && actions.redeploying)
          }
          onClick={
            primary === 'restart'
              ? actions.restart
              : primary === 'retry'
                ? actions.redeploy
                : () => onDeploy('existing-image')
          }
        >
          {primary === 'restart' ? (
            <RotateCwGlassIcon size="1.4em" aria-hidden="true" />
          ) : (
            <RocketGlassIcon size="1.4em" aria-hidden="true" />
          )}
          {primary === 'restart'
            ? actions.restartLabel
            : primary === 'retry'
              ? t('overviewHero.retryDeploy')
              : t('overviewHero.deploy')}
        </Button>
        <ActionMenu
          trigger={
            <Button
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
            id: 'add-tag',
            label: t('overviewHero.addTag'),
            icon: <TagGlassIcon size="1.4em" />,
            onSelect: () => setTagOpen(true),
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
      {tagOpen ? (
        <AddTagDialog
          appName={app.name}
          open={tagOpen}
          onOpenChange={setTagOpen}
        />
      ) : null}
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
