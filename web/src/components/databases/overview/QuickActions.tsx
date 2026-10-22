import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowClockwiseIcon,
  ChartLineIcon,
  CloudArrowUpIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button, buttonVariants } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { engineHasBackups } from '@/lib/databaseCatalog'
import { useTriggerBackup } from '../../../queries/backupHistory'
import { useStartDatabase, useStopDatabase } from '../../../queries/databases'
import type { DatabaseResource } from '../../../types/databaseDetail'
import { ConnectDrawer } from '../ConnectDrawer'

export function QuickActions({ database }: { database: DatabaseResource }) {
  const { t } = useTranslation('databases')
  const name = database.name
  const backup = useTriggerBackup(name)
  const stop = useStopDatabase()
  const start = useStartDatabase()
  const [restarting, setRestarting] = useState(false)
  const targetId = database.backup_target_id
  const running = !database.suspended && !database.provision_state

  if (database.provision_state) return null

  function runBackup() {
    if (!targetId) return
    backup.mutate(targetId, {
      onSuccess: () => {
        toast.add({
          title: t('detail.backupStarted', { name }),
          type: 'success',
        })
      },
      onError: (error) => {
        toast.add({ title: error.message, type: 'error' })
      },
    })
  }

  async function runRestart() {
    setRestarting(true)
    try {
      await stop.mutateAsync(name)
      await start.mutateAsync(name)
      toast.add({ title: t('detail.restarted', { name }), type: 'success' })
    } catch (error) {
      toast.add({
        title: error instanceof Error ? error.message : String(error),
        type: 'error',
      })
    } finally {
      setRestarting(false)
    }
  }

  return (
    <div
      className="flex flex-wrap items-center gap-2"
      role="group"
      aria-label={t('quickActions.label')}
    >
      {running ? <ConnectDrawer database={database} /> : null}
      {running && engineHasBackups(database.engine) && targetId ? (
        <Button
          size="sm"
          variant="outline"
          disabled={backup.isPending}
          onClick={runBackup}
        >
          <CloudArrowUpIcon aria-hidden="true" />
          {backup.isPending
            ? t('detail.backupStarting')
            : t('detail.backupNow')}
        </Button>
      ) : null}
      {engineHasBackups(database.engine) && !targetId ? (
        <Link
          to="/databases/$name/overview"
          params={{ name }}
          hash="backups"
          className={buttonVariants({ size: 'sm', variant: 'outline' })}
        >
          <CloudArrowUpIcon aria-hidden="true" />
          {t('detail.backupSetup')}
        </Link>
      ) : null}
      {running ? (
        <Button
          size="sm"
          variant="outline"
          disabled={restarting}
          onClick={() => {
            void runRestart()
          }}
        >
          <ArrowClockwiseIcon aria-hidden="true" />
          {restarting ? t('detail.restarting') : t('detail.restart')}
        </Button>
      ) : null}
      {running ? (
        <Link
          to="/databases/$name/metrics"
          params={{ name }}
          className={buttonVariants({ size: 'sm', variant: 'outline' })}
        >
          <ChartLineIcon aria-hidden="true" />
          {t('quickActions.metrics')}
        </Link>
      ) : null}
    </div>
  )
}
