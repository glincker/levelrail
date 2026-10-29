import { lazy, Suspense, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  EllipsisGlassIcon,
  PauseGlassIcon,
  PlayGlassIcon,
  RestoreGlassIcon,
  RotateCwGlassIcon,
  TrashGlassIcon,
  UploadGlassIcon,
} from '@thesvg/ui-react'
import { ActionMenu } from '@/components/kit'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useTriggerBackup } from '../../queries/backupHistory'
import { useStartDatabase, useStopDatabase } from '../../queries/databases'
import type { DatabaseResource } from '../../types/databaseDetail'
import { engineHasBackups } from '@/lib/databaseCatalog'
import { DeleteDatabaseDialog } from '../DeleteDatabaseDialog'
import { ConnectDrawer } from './ConnectDrawer'

const BackupSetupDialog = lazy(() =>
  import('./backupSetup/BackupSetupDialog').then((m) => ({
    default: m.BackupSetupDialog,
  })),
)

// One primary action (Start when stopped, otherwise Back up now), the rest in
// the overflow menu so Delete never carries the weight of the primary.
export function DatabaseActionBar({
  database,
}: {
  database: DatabaseResource
}) {
  const { t } = useTranslation('databases')
  const navigate = useNavigate()
  const name = database.name
  const suspended = Boolean(database.suspended)
  const backup = useTriggerBackup(name)
  const stop = useStopDatabase()
  const start = useStartDatabase()
  const [restarting, setRestarting] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [setupOpen, setSetupOpen] = useState(false)
  const targetId = database.backup_target_id

  function onError(error: Error) {
    toast.add({ title: error.message, type: 'error' })
  }

  function runBackup() {
    if (!targetId) return
    backup.mutate(targetId, {
      onSuccess: () => {
        toast.add({
          title: t('detail.backupStarted', { name }),
          type: 'success',
        })
      },
      onError,
    })
  }

  function runStart() {
    start.mutate(name, {
      onSuccess: () => {
        toast.add({ title: t('detail.startToast', { name }), type: 'success' })
      },
      onError,
    })
  }

  function runStop() {
    stop.mutate(name, {
      onSuccess: () => {
        toast.add({ title: t('detail.stopToast', { name }), type: 'success' })
      },
      onError,
    })
  }

  async function runRestart() {
    setRestarting(true)
    try {
      await stop.mutateAsync(name)
      await start.mutateAsync(name)
      toast.add({ title: t('detail.restarted', { name }), type: 'success' })
    } catch (error) {
      onError(error instanceof Error ? error : new Error(String(error)))
    } finally {
      setRestarting(false)
    }
  }

  if (database.provision_state) {
    return (
      <>
        <Button
          size="sm"
          disabled
          title={t('list.pendingActionHint')}
          data-testid="pending-actions-disabled"
        >
          <UploadGlassIcon size="1.4em" aria-hidden="true" />
          {t('detail.backupNow')}
        </Button>
        <p className="text-xs text-muted-foreground">
          {t('list.pendingActionHint')}
        </p>
      </>
    )
  }

  return (
    <>
      <ConnectDrawer database={database} />
      {suspended ? (
        <Button size="sm" disabled={start.isPending} onClick={runStart}>
          <PlayGlassIcon size="1.4em" aria-hidden="true" />
          {t('detail.start')}
        </Button>
      ) : !engineHasBackups(database.engine) ? null : targetId ? (
        <Button size="sm" disabled={backup.isPending} onClick={runBackup}>
          <UploadGlassIcon size="1.4em" aria-hidden="true" />
          {backup.isPending
            ? t('detail.backupStarting')
            : t('detail.backupNow')}
        </Button>
      ) : (
        <Button
          size="sm"
          onClick={() => {
            setSetupOpen(true)
          }}
        >
          <UploadGlassIcon size="1.4em" aria-hidden="true" />
          {t('detail.backupSetup')}
        </Button>
      )}
      {setupOpen ? (
        <Suspense fallback={null}>
          <BackupSetupDialog
            database={database}
            open={setupOpen}
            onOpenChange={setSetupOpen}
          />
        </Suspense>
      ) : null}
      <ActionMenu
        trigger={
          <Button variant="ghost" size="icon-sm" aria-label={t('detail.more')}>
            <EllipsisGlassIcon size="1.4em" aria-hidden="true" />
          </Button>
        }
        items={[
          {
            id: 'restart',
            label: restarting ? t('detail.restarting') : t('detail.restart'),
            icon: <RotateCwGlassIcon size="1.4em" />,
            disabled: restarting || suspended,
            onSelect: () => {
              void runRestart()
            },
          },
          {
            id: 'toggle',
            label: suspended ? t('detail.start') : t('detail.stop'),
            icon: suspended ? (
              <PlayGlassIcon size="1.4em" />
            ) : (
              <PauseGlassIcon size="1.4em" />
            ),
            disabled: start.isPending || stop.isPending || restarting,
            onSelect: suspended ? runStart : runStop,
          },
          {
            id: 'restore',
            label: t('detail.restore'),
            icon: <RestoreGlassIcon size="1.4em" />,
            onSelect: () => {
              void navigate({
                to: '/databases/$name/overview',
                params: { name },
                hash: 'backups',
              })
            },
          },
          {
            id: 'delete',
            label: t('detail.delete'),
            icon: <TrashGlassIcon size="1.4em" />,
            tone: 'danger',
            onSelect: () => {
              setDeleteOpen(true)
            },
          },
        ]}
      />
      <DeleteDatabaseDialog
        name={name}
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
      />
    </>
  )
}
