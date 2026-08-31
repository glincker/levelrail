import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import { PencilSimpleIcon, PlusCircleIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useCreateFeatureFlag, useUpdateFeatureFlag } from '../queries/featureFlags'
import type { FeatureFlag, FeatureFlagRequest } from '../types/featureFlags'

const KEY_REGEX = /^[a-z0-9_-]+$/

const featureFlagSchema = z.object({
  key: z
    .string()
    .trim()
    .min(1, 'Key is required')
    .regex(KEY_REGEX, 'Lowercase letters, digits, hyphens, and underscores only'),
  name: z.string().trim().min(1, 'Name is required'),
  description: z.string().trim(),
  enabled: z.boolean(),
  rolloutPercentage: z.number().min(0).max(100),
})

type FeatureFlagFormValues = z.infer<typeof featureFlagSchema>

const DEFAULT_VALUES: FeatureFlagFormValues = {
  key: '',
  name: '',
  description: '',
  enabled: true,
  rolloutPercentage: 100,
}

export function FeatureFlagDialog({
  appName,
  flag,
}: {
  appName: string
  flag?: FeatureFlag
}) {
  const [open, setOpen] = useState(false)
  const isEdit = flag !== undefined
  const createFlag = useCreateFeatureFlag(appName)
  const updateFlag = useUpdateFeatureFlag(appName)
  const mutation = isEdit ? updateFlag : createFlag

  const defaultValues: FeatureFlagFormValues = flag
    ? {
        key: flag.key,
        name: flag.name,
        description: flag.description ?? '',
        enabled: flag.enabled,
        rolloutPercentage: flag.rollout_percentage,
      }
    : DEFAULT_VALUES

  const { control, register, handleSubmit, formState, reset } = useForm<FeatureFlagFormValues>({
    resolver: zodResolver(featureFlagSchema),
    defaultValues,
  })

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(defaultValues)
      mutation.reset()
    }
  }

  const onSubmit = handleSubmit((values) => {
    const req: FeatureFlagRequest = {
      key: values.key.trim(),
      name: values.name.trim(),
      description: values.description.trim(),
      enabled: values.enabled,
      rollout_percentage: values.rolloutPercentage,
    }
    const onSuccess = () => {
      handleOpenChange(false)
      toast.add({
        title: isEdit ? 'Feature flag updated.' : 'Feature flag created.',
        type: 'success',
      })
    }
    if (isEdit) {
      updateFlag.mutate({ id: flag.id, req }, { onSuccess })
    } else {
      createFlag.mutate(req, { onSuccess })
    }
  })

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger
        render={
          isEdit ? (
            <Button variant="outline" size="sm">
              <PencilSimpleIcon className="size-3.5" aria-hidden="true" />
              Edit
            </Button>
          ) : (
            <Button size="sm">
              <PlusCircleIcon className="size-3.5" aria-hidden="true" />
              Create flag
            </Button>
          )
        }
      />
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5">
            {isEdit ? (
              <PencilSimpleIcon className="size-4 text-muted-foreground" aria-hidden="true" />
            ) : (
              <PlusCircleIcon className="size-4 text-muted-foreground" aria-hidden="true" />
            )}
            {isEdit ? 'Edit feature flag' : 'Create feature flag'}
          </DialogTitle>
          <DialogDescription>
            Read live by your app&apos;s own code via{' '}
            <code>GET /api/v1/flags/evaluate/{'{key}'}</code>. Changes take
            effect immediately, no redeploy or restart needed.
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          <Field>
            <FieldLabel htmlFor="flag-key">Key</FieldLabel>
            <Input
              id="flag-key"
              placeholder="e.g. new-checkout"
              disabled={isEdit}
              {...register('key')}
            />
            <FieldDescription>
              {isEdit
                ? "Can't be changed after create."
                : 'Globally unique across every app on this control plane. Your app looks this up by this exact string.'}
            </FieldDescription>
            <FieldError errors={[formState.errors.key]} />
          </Field>

          <Field>
            <FieldLabel htmlFor="flag-name">Name</FieldLabel>
            <Input id="flag-name" placeholder="e.g. New checkout" {...register('name')} />
            <FieldError errors={[formState.errors.name]} />
          </Field>

          <Field>
            <FieldLabel htmlFor="flag-description">Description</FieldLabel>
            <Textarea
              id="flag-description"
              placeholder="Optional"
              rows={2}
              {...register('description')}
            />
          </Field>

          <Field orientation="horizontal">
            <Controller
              control={control}
              name="enabled"
              render={({ field }) => (
                <Switch
                  id="flag-enabled"
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            />
            <FieldLabel htmlFor="flag-enabled">Enabled</FieldLabel>
          </Field>

          <Field>
            <FieldLabel htmlFor="flag-rollout">Rollout percentage</FieldLabel>
            <Controller
              control={control}
              name="rolloutPercentage"
              render={({ field }) => (
                <Input
                  id="flag-rollout"
                  type="number"
                  min={0}
                  max={100}
                  step={1}
                  value={field.value}
                  onChange={(e) => field.onChange(Number(e.target.value))}
                />
              )}
            />
            <FieldDescription>
              Percent of callers this flag evaluates as on when enabled. Each
              caller lands consistently on the same side of the split across
              repeated calls.
            </FieldDescription>
            <FieldError errors={[formState.errors.rolloutPercentage]} />
          </Field>

          {mutation.isError ? (
            <p className="text-sm text-destructive">{mutation.error.message}</p>
          ) : null}

          <DialogFooter>
            <Button type="submit" disabled={mutation.isPending}>
              {mutation.isPending
                ? isEdit
                  ? 'Saving...'
                  : 'Creating...'
                : isEdit
                  ? 'Save changes'
                  : 'Create flag'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
