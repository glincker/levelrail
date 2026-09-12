import type { Control, FieldValues, FormState, UseFormRegister } from 'react-hook-form'
import { Controller } from 'react-hook-form'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { WEEKDAY_LABEL, type CronFieldsValues } from '../lib/cronSchedule'

// The frequency/time/weekday/custom-cron builder fields, extracted out of
// BackupScheduleFormView so ScheduledTaskDialog can present the exact same
// daily/weekly/custom-cron UI over lib/cronSchedule.ts's grammar without a
// second copy of this markup.
export function CronScheduleFields<T extends FieldValues & CronFieldsValues>({
  idPrefix,
  control,
  register,
  formState,
  frequency,
}: {
  idPrefix: string
  control: Control<T>
  register: UseFormRegister<T>
  formState: FormState<T>
  frequency: CronFieldsValues['frequency']
}) {
  return (
    <>
      <Field>
        <FieldLabel htmlFor={`${idPrefix}-frequency`}>Frequency</FieldLabel>
        <Controller
          control={control}
          name={'frequency' as never}
          render={({ field }) => (
            <Select
              value={field.value}
              onValueChange={(value: string | null) => {
                field.onChange(value ?? 'daily')
              }}
            >
              <SelectTrigger id={`${idPrefix}-frequency`} className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="daily">Daily</SelectItem>
                <SelectItem value="weekly">Weekly</SelectItem>
                <SelectItem value="custom">Custom cron expression</SelectItem>
              </SelectContent>
            </Select>
          )}
        />
      </Field>

      {frequency === 'weekly' ? (
        <Field>
          <FieldLabel htmlFor={`${idPrefix}-weekday`}>Day of week</FieldLabel>
          <Controller
            control={control}
            name={'weekday' as never}
            render={({ field }) => (
              <Select
                value={field.value}
                onValueChange={(value: string | null) => {
                  field.onChange(value ?? '0')
                }}
              >
                <SelectTrigger id={`${idPrefix}-weekday`} className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(WEEKDAY_LABEL).map(([value, label]) => (
                    <SelectItem key={value} value={value}>
                      {label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          />
        </Field>
      ) : null}

      {frequency !== 'custom' ? (
        <Field>
          <FieldLabel htmlFor={`${idPrefix}-time`}>Time</FieldLabel>
          <Input
            id={`${idPrefix}-time`}
            type="time"
            {...register('time' as never)}
          />
          <FieldError errors={[formState.errors.time]} />
        </Field>
      ) : (
        <Field className="sm:col-span-2">
          <FieldLabel htmlFor={`${idPrefix}-cron`}>Cron expression</FieldLabel>
          <Input
            id={`${idPrefix}-cron`}
            placeholder="0 3 * * *"
            className="font-mono"
            {...register('customCron' as never)}
          />
          <FieldDescription>
            Standard 5-field cron: minute hour day-of-month month day-of-week.
          </FieldDescription>
          <FieldError errors={[formState.errors.customCron]} />
        </Field>
      )}
    </>
  )
}
