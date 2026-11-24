import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from '@tanstack/react-router'
import { z } from 'zod'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldError, FieldLabel } from '@/components/ui/field'
import { FormErrorSummary } from '@/components/ui/form-error-summary'
import { useCreateApp } from '../queries/apps'
import { useAppForm } from '../hooks/useAppForm'
import { getProjectScope } from '../lib/projectScope'
import { healthCheckFrom } from '../lib/healthCheckDefaults'
import {
  buildCreateResourceSuccessHandler,
  resolveSubmittedProjectId,
} from '../lib/createResourcePlacement'
import type { GoalDef } from '../lib/createGoals'
import { NO_PROJECT_VALUE } from './PlacementFields'

const schema = z.object({
  image: z.string().trim().min(1),
  name: z
    .string()
    .trim()
    .min(1)
    .regex(/^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/),
  port: z.coerce.number().int().positive(),
  healthPath: z.string().trim(),
})
type FormInput = z.input<typeof schema>
type FormOutput = z.output<typeof schema>

// Name suggestion from the image reference, only while Name is untouched.
function slugFrom(image: string): string {
  const last = image.trim().split('/').filter(Boolean).pop() ?? ''
  return (last.split('@')[0] ?? '')
    .replace(/:[^:]*$/, '')
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

// Prefilled one-row create for an app goal: same useCreateApp as the
// full image form, with port and health path chosen by the goal.
export function CreateGoalAppFields({
  goal,
  open,
  onCreated,
  onAdvanced,
}: {
  goal: GoalDef
  open: boolean
  onCreated: () => void
  /** Hands over to the full image form for node, strategy and replicas. */
  onAdvanced: () => void
}) {
  const { t } = useTranslation('databases')
  const navigate = useNavigate()
  const createApp = useCreateApp()
  const defaults: FormInput = {
    image: '',
    name: '',
    port: String(goal.defaults?.port ?? 8080),
    healthPath: goal.defaults?.healthPath ?? '',
  }
  const form = useAppForm<FormInput, FormOutput>(schema, {
    defaultValues: defaults,
  })
  const { register, handleSubmit, formState, reset, setValue, getValues } = form

  useEffect(() => {
    if (!open) {
      reset(defaults)
      createApp.reset()
    }
    // Only the dialog's open transition should reset the form.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  const onSubmit = handleSubmit((values) => {
    const project = getProjectScope() || NO_PROJECT_VALUE
    createApp.mutate(
      {
        name: values.name,
        image: values.image,
        port: values.port,
        health: healthCheckFrom(values.healthPath !== '', values.healthPath),
        project_id: resolveSubmittedProjectId(project),
      },
      {
        onSuccess: buildCreateResourceSuccessHandler({
          resourceLabel: 'App',
          clearDraft: () => undefined,
          onCreated,
          onNavigate: (name) => {
            void navigate({ to: '/apps/$name', params: { name } })
          },
        }),
      },
    )
  })

  const imageField = register('image')

  return (
    <form
      onSubmit={(e) => {
        void onSubmit(e)
      }}
      noValidate
      className="space-y-3"
    >
      <FormErrorSummary
        form={form}
        fields={{
          image: { label: t('wizard.goals.fields.image'), id: 'goal-image' },
          name: { label: t('wizard.goals.fields.name'), id: 'goal-name' },
          port: { label: t('wizard.goals.fields.port'), id: 'goal-port' },
        }}
      />
      <div className="grid gap-3 sm:grid-cols-[2fr_1.2fr_6rem_1fr]">
        <Field>
          <FieldLabel htmlFor="goal-image">
            {t('wizard.goals.fields.image')}
          </FieldLabel>
          <Input
            id="goal-image"
            className="font-mono"
            placeholder={t('wizard.goals.fields.imagePlaceholder')}
            {...imageField}
            onChange={(e) => {
              void imageField.onChange(e)
              if (!getValues('name').trim()) {
                setValue('name', slugFrom(e.target.value), {
                  shouldValidate: false,
                })
              }
            }}
          />
          <FieldError errors={[formState.errors.image]} />
        </Field>
        <Field>
          <FieldLabel htmlFor="goal-name">
            {t('wizard.goals.fields.name')}
          </FieldLabel>
          <Input id="goal-name" {...register('name')} />
          <FieldError errors={[formState.errors.name]} />
        </Field>
        <Field>
          <FieldLabel htmlFor="goal-port">
            {t('wizard.goals.fields.port')}
          </FieldLabel>
          <Input
            id="goal-port"
            type="number"
            min="1"
            step="1"
            {...register('port')}
          />
          <FieldError errors={[formState.errors.port]} />
        </Field>
        <Field>
          <FieldLabel htmlFor="goal-health">
            {t('wizard.goals.fields.health')}
          </FieldLabel>
          <Input
            id="goal-health"
            className="font-mono"
            placeholder={t('wizard.goals.healthOff')}
            {...register('healthPath')}
          />
        </Field>
      </div>
      {createApp.error ? (
        <p role="alert" className="text-sm text-destructive">
          {createApp.error.message}
        </p>
      ) : null}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <Button type="button" variant="ghost" size="sm" onClick={onAdvanced}>
          {t('wizard.goals.advanced')}
        </Button>
        <Button type="submit" disabled={createApp.isPending}>
          {createApp.isPending
            ? t('wizard.goals.deploying')
            : t('wizard.goals.deploy')}
        </Button>
      </div>
    </form>
  )
}
