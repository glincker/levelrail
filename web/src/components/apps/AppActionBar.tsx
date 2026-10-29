import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CopyGlassIcon,
  EllipsisGlassIcon,
  KeyGlassIcon,
  LinkGlassIcon,
  LoaderGlassIcon,
  PauseGlassIcon,
  PlayGlassIcon,
  RocketGlassIcon,
  RotateCwGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { ActionMenu } from '@/components/kit'
import { AppAccessDetailsDialog } from '../templates/AppAccessDetails'
import { hasTemplateAccess } from '../templates/templateAccess'
import { Button } from '@/components/ui/button'
import { CloneAppDialog } from '../CloneAppDialog'
import { DeleteAppDialog } from '../DeleteAppDialog'
import { PromoteAppDialog } from '../PromoteAppDialog'
import { useOverviewActions } from '../overview/useOverviewActions'
import { useAppInMaintenance } from '../../hooks/useAppInMaintenance'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { DeployTab } from '../overview/DeploySheet'
import type { AppDetail } from '../../types/appDetail'
import { useAppAsleep, useAppSleepActions } from '../../queries/appSleep'
import { useGitSource } from '../../queries/gitSources'
import { DeployLinkDialog } from '../newapp/DeployLinkDialog'

// One primary action, Restart beside it, everything else behind the overflow
// menu so Delete never carries the same visual weight as Deploy.
export function AppActionBar({
  app,
  onDeploy,
}: {
  app: AppDetail
  onDeploy: (tab: DeployTab) => void
}) {
  const { t } = useTranslation('common')
  const actions = useOverviewActions(app, null)
  const asleep = useAppAsleep(app.name)
  const { wake } = useAppSleepActions(app.name)
  const [promoteOpen, setPromoteOpen] = useState(false)
  const [cloneOpen, setCloneOpen] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [linkOpen, setLinkOpen] = useState(false)
  const [accessOpen, setAccessOpen] = useState(false)
  const { t: tt } = useTranslation('templates')
  const gitSource = useGitSource(app.name).data
  const { t: tn } = useTranslation('newApp')
  const { t: tm } = useTranslation('maintenance')
  const inMaintenance = useAppInMaintenance(app.name)
  const deployButton = (
    <Button
      size="sm"
      onClick={() => {
        onDeploy('existing-image')
      }}
    >
      <RocketGlassIcon size="1.2em" aria-hidden="true" />
      {t('appActions.deploy', { defaultValue: 'Deploy' })}
    </Button>
  )

  return (
    <>
      {inMaintenance ? (
        <Tooltip>
          <TooltipTrigger render={deployButton} />
          <TooltipContent>{tm('deployNotice.tooltip')}</TooltipContent>
        </Tooltip>
      ) : (
        deployButton
      )}
      <Button
        size="sm"
        variant="outline"
        disabled={actions.restarting}
        onClick={actions.restart}
      >
        {actions.restarting || actions.slowRestart ? (
          <LoaderGlassIcon
            size="1.2em"
            aria-hidden="true"
            className="animate-spin motion-reduce:animate-none"
          />
        ) : (
          <RotateCwGlassIcon size="1.2em" aria-hidden="true" />
        )}
        <span className="max-md:sr-only">{actions.restartLabel}</span>
      </Button>
      {actions.confirmDialog}
      <ActionMenu
        trigger={
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('appActions.more', {
              defaultValue: 'More actions',
            })}
          >
            <EllipsisGlassIcon size="1.2em" aria-hidden="true" />
          </Button>
        }
        items={[
          {
            id: 'redeploy',
            label: actions.redeploying
              ? t('appActions.redeploying', { defaultValue: 'Redeploying...' })
              : t('appActions.redeploy', { defaultValue: 'Redeploy current' }),
            icon: <RocketGlassIcon size="1.2em" />,
            disabled: actions.redeploying,
            onSelect: actions.redeploy,
          },
          {
            id: 'toggle',
            label: asleep
              ? t('appActions.wake', { defaultValue: 'Wake now' })
              : app.suspended
                ? t('appActions.start', { defaultValue: 'Start' })
                : actions.stopping
                  ? t('appPower.stopping')
                  : t('appActions.stop', { defaultValue: 'Stop' }),
            icon: app.suspended ? (
              <PlayGlassIcon size="1.2em" />
            ) : (
              <PauseGlassIcon size="1.2em" />
            ),
            disabled: asleep ? wake.isPending : actions.togglingRunning,
            onSelect: asleep ? () => wake.mutate() : actions.toggleRunning,
          },
          {
            id: 'promote',
            label: t('appActions.promote', { defaultValue: 'Promote...' }),
            icon: <RocketGlassIcon size="1.2em" />,
            onSelect: () => {
              setPromoteOpen(true)
            },
          },
          {
            id: 'clone',
            label: t('appActions.clone', { defaultValue: 'Clone...' }),
            icon: <CopyGlassIcon size="1.2em" />,
            onSelect: () => {
              setCloneOpen(true)
            },
          },
          ...(hasTemplateAccess(app)
            ? [
                {
                  id: 'access-details',
                  label: tt('access.open'),
                  icon: <KeyGlassIcon size="1.2em" />,
                  onSelect: () => {
                    setAccessOpen(true)
                  },
                },
              ]
            : []),
          ...(gitSource
            ? [
                {
                  id: 'deploy-link',
                  label: tn('deployLinkDialog.action'),
                  icon: <LinkGlassIcon size="1.2em" />,
                  onSelect: () => {
                    setLinkOpen(true)
                  },
                },
              ]
            : []),
          {
            id: 'delete',
            label: t('appActions.delete', { defaultValue: 'Delete...' }),
            icon: <TrashGlassIcon size="1.2em" />,
            tone: 'danger',
            onSelect: () => {
              setDeleteOpen(true)
            },
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
      <AppAccessDetailsDialog
        app={app}
        open={accessOpen}
        onOpenChange={setAccessOpen}
      />
      {linkOpen && gitSource ? (
        <DeployLinkDialog
          open
          onOpenChange={setLinkOpen}
          repo={gitSource.repo_url}
          branch={gitSource.branch}
          name={app.name}
        />
      ) : null}
      <DeleteAppDialog
        name={app.name}
        control={{
          open: deleteOpen,
          onOpenChange: setDeleteOpen,
          hideTrigger: true,
        }}
      />
    </>
  )
}
