import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { useBackupDrills } from '../../queries/backupProtection'
import { useStartDatabaseRestoreTest } from '../../queries/databaseRestoreTest'
import { restoreTestSummary } from './restoreTestSummary'

// Restores the newest backup into a throwaway container; never the live database.
export function RestoreTestRow({ databaseName }: { databaseName: string }) {
  const { t } = useTranslation('backups')
  const drills = useBackupDrills({ database: databaseName, limit: 1 })
  const start = useStartDatabaseRestoreTest(databaseName)
  const latest = drills.data?.[0]
  const running = latest?.status === 'running' || start.isPending

  if (drills.isLoading) return <Skeleton className="h-9 w-full" />

  function handleStart() {
    start.mutate(undefined, {
      onSuccess: () =>
        toast.add({
          title: t('restoreTest.startedTitle'),
          description: t('restoreTest.startedBody'),
          type: 'success',
        }),
      onError: (error) =>
        toast.add({
          title: t('restoreTest.startFailed'),
          description: error.message,
          type: 'error',
        }),
    })
  }

  return (
    <div className="flex flex-wrap items-center justify-between gap-2">
      <p
        className={
          latest?.status === 'failed'
            ? 'text-sm text-destructive'
            : 'text-sm text-muted-foreground'
        }
      >
        {restoreTestSummary(t, latest)}
      </p>
      <Button
        variant="outline"
        size="sm"
        disabled={running}
        onClick={handleStart}
      >
        {t('restoreTest.button')}
      </Button>
    </div>
  )
}
