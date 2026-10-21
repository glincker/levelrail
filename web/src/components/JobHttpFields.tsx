import type { Control, FormState, UseFormRegister } from 'react-hook-form'
import { Controller } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import type { JobFormValues } from '../lib/jobForm'
import type { JobHttpMethod } from '../types/scheduledTasks'

const METHODS: JobHttpMethod[] = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE']

// JobHttpFields is the request an http job sends: the control plane wakes
// the app if it sleeps, then calls this path on schedule.
export function JobHttpFields({
  control,
  register,
  formState,
}: {
  control: Control<JobFormValues>
  register: UseFormRegister<JobFormValues>
  formState: FormState<JobFormValues>
}) {
  const { t } = useTranslation('jobs')
  return (
    <div className="space-y-4">
      <Field>
        <FieldLabel htmlFor="job-http-method">
          {t('dialog.http.method')}
        </FieldLabel>
        <Controller
          control={control}
          name="httpMethod"
          render={({ field }) => (
            <Select<JobHttpMethod>
              value={field.value}
              onValueChange={(value) => {
                field.onChange(value ?? 'POST')
              }}
            >
              <SelectTrigger id="job-http-method" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {METHODS.map((m) => (
                  <SelectItem key={m} value={m}>
                    {m}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor="job-http-path">{t('dialog.http.path')}</FieldLabel>
        <Input
          id="job-http-path"
          placeholder={t('dialog.http.pathPlaceholder')}
          {...register('httpPath')}
        />
        <FieldDescription>{t('dialog.http.pathHint')}</FieldDescription>
        <FieldError
          errors={[
            formState.errors.httpPath && {
              message: t('dialog.http.pathInvalid'),
            },
          ]}
        />
      </Field>
      <Field>
        <FieldLabel htmlFor="job-http-body">{t('dialog.http.body')}</FieldLabel>
        <Input id="job-http-body" {...register('httpBody')} />
        <FieldDescription>{t('dialog.http.bodyHint')}</FieldDescription>
      </Field>
      <Field orientation="horizontal">
        <Controller
          control={control}
          name="httpBackground"
          render={({ field }) => (
            <Switch
              id="job-http-background"
              checked={field.value}
              onCheckedChange={field.onChange}
            />
          )}
        />
        <FieldLabel htmlFor="job-http-background">
          {t('dialog.http.background')}
        </FieldLabel>
      </Field>
      <FieldDescription>{t('dialog.http.backgroundHint')}</FieldDescription>
    </div>
  )
}
