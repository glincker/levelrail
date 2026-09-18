import { Controller, type Control, type FieldErrors } from 'react-hook-form'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { DurationInput } from '@/components/ui/duration-input'
import type { BackupResourceKind } from '../types/alerts'
import type { AppVolume } from '../types/appDetail'
import type { DatabaseResource } from '../types/databaseDetail'
import type { BackupMissingFormShape } from './backupMissingAlertRule'

const BACKUP_RESOURCE_KIND_OPTIONS: { value: BackupResourceKind; label: string }[] = [
  { value: 'database', label: 'Database' },
  { value: 'volume', label: 'App volume' },
]

interface BackupMissingFieldsProps {
  idPrefix: string
  control: Control<BackupMissingFormShape>
  errors: FieldErrors<BackupMissingFormShape>
  backupResourceKind: string
  databases: DatabaseResource[]
  volumes: AppVolume[] | undefined
}

// Shared by CreateAlertRuleDialog and EditAlertRuleDialog: the
// backup_missing-kind fields (watch target, database/volume picker,
// grace period) render identically in both, differing only in the
// `idPrefix` used for element ids/labels. Callers cast their own,
// larger form's control/errors down to BackupMissingFormShape since
// that shape is a strict subset of both dialogs' full schemas.
export function BackupMissingFields({
  idPrefix,
  control,
  errors,
  backupResourceKind,
  databases,
  volumes,
}: BackupMissingFieldsProps) {
  return (
    <>
      <Field>
        <FieldLabel htmlFor={`${idPrefix}-backup-resource-kind`}>Watches</FieldLabel>
        <Controller
          control={control}
          name="backupResourceKind"
          render={({ field }) => (
            <Select value={field.value} onValueChange={field.onChange}>
              <SelectTrigger id={`${idPrefix}-backup-resource-kind`} className="w-full">
                <SelectValue placeholder="Choose what to watch" />
              </SelectTrigger>
              <SelectContent>
                {BACKUP_RESOURCE_KIND_OPTIONS.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
        <FieldError errors={[errors.backupResourceKind]} />
      </Field>

      {backupResourceKind === 'database' ? (
        <Field>
          <FieldLabel htmlFor={`${idPrefix}-backup-database`}>Database</FieldLabel>
          {databases.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No databases yet. Create one from the Databases page first.
            </p>
          ) : (
            <Controller
              control={control}
              name="backupDatabaseName"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger id={`${idPrefix}-backup-database`} className="w-full">
                    <SelectValue placeholder="Choose a database" />
                  </SelectTrigger>
                  <SelectContent>
                    {databases.map((db) => (
                      <SelectItem key={db.name} value={db.name}>
                        {db.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          )}
          <FieldError errors={[errors.backupDatabaseName]} />
        </Field>
      ) : backupResourceKind === 'volume' ? (
        <Field>
          <FieldLabel htmlFor={`${idPrefix}-backup-volume`}>Volume</FieldLabel>
          {!volumes || volumes.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              This app has no named volumes declared in app.yaml.
            </p>
          ) : (
            <Controller
              control={control}
              name="backupVolumeName"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger id={`${idPrefix}-backup-volume`} className="w-full">
                    <SelectValue placeholder="Choose a volume" />
                  </SelectTrigger>
                  <SelectContent>
                    {volumes.map((v) => (
                      <SelectItem key={v.name} value={v.name}>
                        {v.name} ({v.container_path})
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          )}
          <FieldError errors={[errors.backupVolumeName]} />
        </Field>
      ) : null}

      <Field>
        <FieldLabel htmlFor={`${idPrefix}-backup-missing-for-duration`}>
          Overdue grace period (optional)
        </FieldLabel>
        <Controller
          control={control}
          name="forDuration"
          render={({ field }) => (
            <DurationInput
              id={`${idPrefix}-backup-missing-for-duration`}
              value={field.value}
              onChange={field.onChange}
              onBlur={field.onBlur}
            />
          )}
        />
        <FieldDescription>
          How long the last successful backup can trail its own schedule
          before this fires. Leave blank to use the control plane&apos;s
          default (6h).
        </FieldDescription>
        <FieldError errors={[errors.forDuration]} />
      </Field>
    </>
  )
}
