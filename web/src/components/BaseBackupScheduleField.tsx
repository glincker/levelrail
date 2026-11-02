import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { useSetBaseBackupSchedule } from '../queries/pitr'

// Own cron for physical base backups, independent of the dump schedule.
export function BaseBackupScheduleField({
  databaseName,
  schedule,
  targetId,
}: {
  databaseName: string
  schedule: string
  targetId: string
}) {
  const { t } = useTranslation('backups')
  const [cron, setCron] = useState(schedule)
  const [target, setTarget] = useState(targetId)
  const save = useSetBaseBackupSchedule(databaseName)

  return (
    <div className="flex flex-col gap-2">
      <h3 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
        {t('baseSchedule.title')}
      </h3>
      <p className="text-sm text-muted-foreground">
        {t('baseSchedule.description')}
      </p>
      <div className="flex flex-wrap items-end gap-3">
        <Field className="w-40">
          <FieldLabel htmlFor="base-backup-cron">
            {t('baseSchedule.cronLabel')}
          </FieldLabel>
          <Input
            id="base-backup-cron"
            value={cron}
            placeholder={t('baseSchedule.cronPlaceholder')}
            onChange={(e) => setCron(e.target.value)}
          />
        </Field>
        <Field className="w-56">
          <FieldLabel htmlFor="base-backup-target">
            {t('baseSchedule.targetLabel')}
          </FieldLabel>
          <Input
            id="base-backup-target"
            value={target}
            placeholder={t('baseSchedule.targetPlaceholder')}
            onChange={(e) => setTarget(e.target.value)}
          />
        </Field>
        <Button
          type="button"
          size="sm"
          disabled={save.isPending}
          onClick={() =>
            save.mutate(
              { schedule: cron.trim(), targetId: target.trim() },
              {
                onSuccess: () =>
                  toast.add({
                    title: t('baseSchedule.saved'),
                    type: 'success',
                  }),
              },
            )
          }
        >
          {save.isPending ? t('baseSchedule.saving') : t('baseSchedule.save')}
        </Button>
      </div>
      {save.isError ? (
        <FieldDescription className="text-destructive">
          {save.error.message}
        </FieldDescription>
      ) : null}
    </div>
  )
}
