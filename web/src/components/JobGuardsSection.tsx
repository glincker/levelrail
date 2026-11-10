import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ShieldCheckGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import { useUpdateScheduledTask } from '../queries/scheduledTasks'
import type { ScheduledTask } from '../types/scheduledTasks'
import { jobErrorMessage } from '../lib/jobAccess'
import {
  PRIMARY_OVERLAP_CHOICES,
  guardsFromTask,
  guardsPatch,
  invalidGuardFields,
  type JobGuardsField,
  type JobGuardsValues,
  type JobOverlapChoice,
} from '../lib/jobGuards'

// Guard rails save on their own through a partial PUT, so they never
// overwrite the rest of the job with stale form values.
export default function JobGuardsSection({
  appName,
  task,
}: {
  appName: string
  task: ScheduledTask
}) {
  const { t } = useTranslation('jobs')
  const initial = guardsFromTask(task)
  const [values, setValues] = useState<JobGuardsValues>(initial)
  const [showErrors, setShowErrors] = useState(false)
  const update = useUpdateScheduledTask(appName)
  const invalid = invalidGuardFields(values)
  const patch = guardsPatch(initial, values)
  const dirty = Object.keys(patch).length > 0
  const choices: JobOverlapChoice[] = PRIMARY_OVERLAP_CHOICES.includes(
    initial.overlap,
  )
    ? PRIMARY_OVERLAP_CHOICES
    : [...PRIMARY_OVERLAP_CHOICES, initial.overlap]

  function set<K extends keyof JobGuardsValues>(
    key: K,
    value: JobGuardsValues[K],
  ) {
    setValues((prev) => ({ ...prev, [key]: value }))
  }

  function save() {
    if (invalid.length > 0) {
      setShowErrors(true)
      return
    }
    update.mutate(
      { id: task.id, req: patch },
      {
        onSuccess: () => {
          toast.add({ title: t('guards.saved'), type: 'success' })
        },
      },
    )
  }

  const countField = (id: string, key: JobGuardsField, hint: string) => (
    <Field>
      <FieldLabel htmlFor={id}>{t(`guards.${key}`)}</FieldLabel>
      <Input
        id={id}
        inputMode="numeric"
        placeholder={t('guards.defaultPlaceholder')}
        value={values[key]}
        aria-invalid={showErrors && invalid.includes(key)}
        onChange={(e) => {
          set(key, e.target.value)
        }}
      />
      <FieldDescription>{hint}</FieldDescription>
      <FieldError
        errors={[
          showErrors && invalid.includes(key)
            ? { message: t('dialog.wholeNumber') }
            : undefined,
        ]}
      />
    </Field>
  )

  return (
    <section
      aria-labelledby="job-guards-title"
      className="space-y-4 border-t border-border px-4 pt-4 pb-4"
    >
      <div className="flex items-center gap-2">
        <ShieldCheckGlassIcon
          className="size-5 text-muted-foreground"
          aria-hidden="true"
        />
        <h3 id="job-guards-title" className="text-sm font-medium">
          {t('guards.title')}
        </h3>
      </div>
      <p className="text-sm text-muted-foreground">{t('guards.description')}</p>

      <div className="grid gap-4 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor="job-guards-overlap">
            {t('dialog.concurrencyLabel')}
          </FieldLabel>
          <Select<JobOverlapChoice>
            value={values.overlap}
            onValueChange={(value) => {
              set('overlap', value ?? initial.overlap)
            }}
          >
            <SelectTrigger id="job-guards-overlap" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {choices.map((c) => (
                <SelectItem key={c} value={c}>
                  {t(`guards.overlap.${c}`)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <FieldDescription>
            {t(`guards.overlapHint.${values.overlap}`)}
          </FieldDescription>
        </Field>
        {countField(
          'job-guards-timeout',
          'timeoutSeconds',
          t('dialog.timeoutHint'),
        )}
        {countField('job-guards-keep-runs', 'keepRuns', t('guards.keepHint'))}
        {countField('job-guards-keep-days', 'keepDays', t('guards.keepHint'))}
      </div>

      <Field orientation="horizontal">
        <Switch
          id="job-guards-alert"
          checked={values.alertOnFailure}
          onCheckedChange={(checked) => {
            set('alertOnFailure', checked)
          }}
        />
        <FieldLabel htmlFor="job-guards-alert">
          {t('guards.alertOnFailure')}
        </FieldLabel>
      </Field>
      <p className="text-xs text-muted-foreground">
        {t('guards.alertOnFailureHint')}
      </p>

      {update.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {jobErrorMessage(update.error, t('accessRule'))}
        </p>
      ) : null}

      <div className="flex justify-end">
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={!dirty || update.isPending}
          onClick={save}
        >
          {update.isPending ? t('dialog.saving') : t('guards.save')}
        </Button>
      </div>
    </section>
  )
}
