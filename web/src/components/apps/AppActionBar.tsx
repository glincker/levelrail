import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowClockwiseIcon,
  CopyIcon,
  DotsThreeIcon,
  LinkIcon,
  PauseIcon,
  PlayIcon,
  RocketLaunchIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { ActionMenu } from '@/components/kit'
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
      <RocketLaunchIcon aria-hidden="true" />
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
        <ArrowClockwiseIcon aria-hidden="true" />
        <span className="max-md:sr-only">
          {actions.restarting
            ? t('appActions.restarting', { defaultValue: 'Restarting...' })
            : t('appActions.restart', { defaultValue: 'Restart' })}
        </span>
      </Button>
      <ActionMenu
        trigger={
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('appActions.more', {
              defaultValue: 'More actions',
            })}
          >
            <DotsThreeIcon aria-hidden="true" />
          </Button>
        }
        items={[
          {
            id: 'redeploy',
            label: actions.redeploying
              ? t('appActions.redeploying', { defaultValue: 'Redeploying...' })
              : t('appActions.redeploy', { defaultValue: 'Redeploy current' }),
            icon: <RocketLaunchIcon />,
            disabled: actions.redeploying,
            onSelect: actions.redeploy,
          },
          {
            id: 'toggle',
            label: asleep
              ? t('appActions.wake', { defaultValue: 'Wake now' })
              : app.suspended
                ? t('appActions.start', { defaultValue: 'Start' })
                : t('appActions.stop', { defaultValue: 'Stop' }),
            icon: app.suspended ? <PlayIcon /> : <PauseIcon />,
            disabled: asleep ? wake.isPending : actions.togglingRunning,
            onSelect: asleep ? () => wake.mutate() : actions.toggleRunning,
          },
          {
            id: 'promote',
            label: t('appActions.promote', { defaultValue: 'Promote...' }),
            icon: <RocketLaunchIcon />,
            onSelect: () => {
              setPromoteOpen(true)
            },
          },
          {
            id: 'clone',
            label: t('appActions.clone', { defaultValue: 'Clone...' }),
            icon: <CopyIcon />,
            onSelect: () => {
              setCloneOpen(true)
            },
          },
          ...(gitSource
            ? [
                {
                  id: 'deploy-link',
                  label: tn('deployLinkDialog.action'),
                  icon: <LinkIcon />,
                  onSelect: () => {
                    setLinkOpen(true)
                  },
                },
              ]
            : []),
          {
            id: 'delete',
            label: t('appActions.delete', { defaultValue: 'Delete...' }),
            icon: <TrashIcon />,
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
