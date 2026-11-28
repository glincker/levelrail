import { useState } from 'react'
import { Link, useNavigate } from '@tanstack/react-router'
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
import { useIsStaticSite } from '../../queries/staticSites'
import type { AppDetail } from '../../types/appDetail'
import type { DeployTab } from './DeploySheet'
import { useOverviewActions } from './useOverviewActions'
import { useUrlReachability } from './useUrlReachability'
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
  const isStatic = useIsStaticSite(app.name)
  const navigate = useNavigate()
  // A static site has no container to restart; its primary is always a redeploy.
  const effectivePrimary =
    isStatic && primary === 'restart' ? 'deploy' : primary
  const [promoteOpen, setPromoteOpen] = useState(false)
  const [cloneOpen, setCloneOpen] = useState(false)
  const [saveAsTemplateOpen, setSaveAsTemplateOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [tagOpen, setTagOpen] = useState(false)
  const { t } = useTranslation('common')
  const { t: tOverview } = useTranslation('appOverview')
  const { reach } = useUrlReachability(app, url)

  return (
    <div className="flex max-w-full shrink-0 flex-wrap items-center gap-2">
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
      {url && reach.state === 'down' ? (
        <span title={tOverview(`status.reason.${reach.state}`)}>
          <Button
            variant="outline"
            size="lg"
            className="gap-2"
            disabled
            aria-description={tOverview(`status.reason.${reach.state}`)}
          >
            <ExternalLinkGlassIcon size="1.4em" aria-hidden="true" />
            <span className="max-sm:sr-only">{t('overviewHero.openApp')}</span>
          </Button>
        </span>
      ) : url ? (
        <a
          href={url}
          target="_blank"
          rel="noreferrer"
          title={
            reach.state === 'waiting_dns' || reach.state === 'unknown'
              ? tOverview(`status.reason.${reach.state}`)
              : undefined
          }
          className={cn(
            buttonVariants({ variant: 'outline', size: 'lg' }),
            'gap-2',
          )}
        >
          <ExternalLinkGlassIcon size="1.4em" aria-hidden="true" />
          <span className="max-sm:sr-only">
            {reach.state === 'waiting_dns'
              ? tOverview('status.openAnyway')
              : t('overviewHero.openApp')}
          </span>
          {isStatic ? null : (
            <span className="max-lg:hidden">
              <Kbd keys={['O']} />
            </span>
          )}
        </a>
      ) : null}

      <div className="flex items-center">
        <Button
          size="lg"
          className="rounded-r-none"
          disabled={
            (effectivePrimary === 'restart' && actions.restarting) ||
            (effectivePrimary === 'retry' && actions.redeploying)
          }
          onClick={
            effectivePrimary === 'restart'
              ? actions.restart
              : effectivePrimary === 'retry'
                ? actions.redeploy
                : () => onDeploy('existing-image')
          }
        >
          {effectivePrimary === 'restart' ? (
            <RotateCwGlassIcon size="1.4em" aria-hidden="true" />
          ) : (
            <RocketGlassIcon size="1.4em" aria-hidden="true" />
          )}
          {effectivePrimary === 'restart'
            ? actions.restartLabel
            : effectivePrimary === 'retry'
              ? t('overviewHero.retryDeploy')
              : t('overviewHero.deploy')}
        </Button>
        <ActionMenu
          trigger={
            <Button
              size="icon-lg"
              className="h-10 w-9 rounded-l-none border-l border-primary-foreground/25"
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
            ...(isStatic
              ? []
              : [
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
                ]),
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
          <Button
            variant="ghost"
            size="icon-lg"
            className="size-10"
            aria-label="More actions"
          >
            <EllipsisGlassIcon size="1.4em" aria-hidden="true" />
          </Button>
        }
        items={[
          ...(isStatic
            ? [
                {
                  id: 'manage-pages',
                  label: t('appActions.manageInPages'),
                  icon: <ExternalLinkGlassIcon size="1.4em" />,
                  onSelect: () => {
                    void navigate({
                      to: '/pages/$name',
                      params: { name: app.name },
                    })
                  },
                },
              ]
            : [
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
              ]),
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
