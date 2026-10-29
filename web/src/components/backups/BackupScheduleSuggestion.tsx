import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { useBackupCoverage } from '../../queries/backupCoverage'
import { useBackupTargetsOptional } from '../../queries/backupTargets'
import { useSetDatabaseBackupSchedule } from '../../queries/databases'
import type { DatabaseResource } from '../../types/databaseDetail'

/** BackupScheduleSuggestion recommends the default schedule for a database that has none, once a destination exists. It never applies itself. */
export function BackupScheduleSuggestion({
  database,
}: {
  database: DatabaseResource
}) {
  const { t } = useTranslation('backups')
  const targets = useBackupTargetsOptional().data ?? []
  const defaults = useBackupCoverage().data?.defaults
  const setSchedule = useSetDatabaseBackupSchedule()
  const target = targets[0]

  if (database.backup_schedule || !target || !defaults) return null

  return (
    <Alert>
      <ShieldCheckGlassIcon
        className="text-tone-success"
        size="1.4em"
        aria-hidden="true"
      />
      <AlertTitle>{t('suggestion.title')}</AlertTitle>
      <AlertDescription className="space-y-2">
        <p>
          {t('suggestion.description', {
            schedule: defaults.schedule,
            retain: defaults.retain,
          })}
        </p>
        <Button
          type="button"
          size="sm"
          disabled={setSchedule.isPending}
          onClick={() => {
            setSchedule.mutate(
              {
                name: database.name,
                targetId: target.id,
                schedule: defaults.schedule,
                retain: defaults.retain,
                retainDays: defaults.retain_days,
              },
              {
                onSuccess: () => {
                  toast.add({ title: t('suggestion.applied'), type: 'success' })
                },
                onError: (err) => {
                  toast.add({
                    title: t('suggestion.failed'),
                    description: err.message,
                    type: 'error',
                  })
                },
              },
            )
          }}
        >
          {t('suggestion.apply')}
        </Button>
      </AlertDescription>
    </Alert>
  )
}
