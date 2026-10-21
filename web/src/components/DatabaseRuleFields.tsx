import { Controller, type Control, type FieldErrors } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import type { DatabaseResource } from '../types/databaseDetail'
import {
  databaseHint,
  type DatabaseRuleFormShape,
} from '@/lib/databaseRuleKinds'
import { AlertKindGuidance } from './AlertKindGuidance'
import { isAlertKindCopyKind } from './alertKindCopy'

/** Fields for the database_* rule kinds: which database, and a threshold where the kind has one. */
export function DatabaseRuleFields({
  idPrefix,
  kind,
  control,
  errors,
  register,
  databases,
}: {
  idPrefix: string
  kind: string
  control: Control<DatabaseRuleFormShape>
  errors: FieldErrors<DatabaseRuleFormShape>
  register: (name: 'threshold') => Record<string, unknown>
  databases: DatabaseResource[]
}) {
  const { t } = useTranslation('observability')
  const hasThreshold = kind !== 'database_down'
  return (
    <>
      <Field>
        <FieldLabel htmlFor={`${idPrefix}-database`}>
          {t('databaseRule.database')}
        </FieldLabel>
        {databases.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('databaseRule.noDatabases')}
          </p>
        ) : (
          <Controller
            control={control}
            name="backupDatabaseName"
            render={({ field }) => (
              <Select value={field.value} onValueChange={field.onChange}>
                <SelectTrigger id={`${idPrefix}-database`} className="w-full">
                  <SelectValue placeholder={t('databaseRule.choose')} />
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
      {hasThreshold ? (
        <Field>
          <FieldLabel htmlFor={`${idPrefix}-db-threshold`}>
            {kind === 'database_backup_stale'
              ? t('databaseRule.thresholdHours')
              : t('databaseRule.thresholdPercent')}
          </FieldLabel>
          <Input
            id={`${idPrefix}-db-threshold`}
            type="number"
            step="any"
            min="0"
            {...register('threshold')}
          />
          <FieldDescription>{databaseHint(t, kind)}</FieldDescription>
          <FieldError errors={[errors.threshold]} />
        </Field>
      ) : (
        <p className="text-sm text-muted-foreground">
          {t('databaseRule.hint.database_down')}
        </p>
      )}
      {isAlertKindCopyKind(kind) ? (
        <AlertKindGuidance kind={kind} withDescription={false} />
      ) : null}
    </>
  )
}
