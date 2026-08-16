import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import {
  GaugeIcon,
  PlusCircleIcon,
  ArrowCounterClockwiseIcon,
} from '@phosphor-icons/react/dist/ssr'
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
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Field, FieldDescription, FieldError, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'
import { useCreateAlertRule } from '../queries/alerts'
import { useNotificationChannelsOptional } from '../queries/notificationChannels'
import { CHANNEL_KIND_LABEL } from './notificationChannelKind'
import type {
  AlertRuleKind,
  Comparator,
  CreateAlertRuleRequest,
} from '../types/alerts'

// Sanity-check regex for a Go `time.Duration` string ("2m", "30s",
// "1h30m", "500ms"), not a real parser: `time.ParseDuration` only runs
// server-side (internal/api/alerts.go's parseOptionalDuration), so this
// is the same class of "catch the obviously wrong input before a round
// trip" check the task asked for, not a guarantee the server will
// accept it. The server's own error message is still surfaced on submit
// failure below for whatever this regex lets through that the server
// rejects anyway.
const GO_DURATION_REGEX = /^-?(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/

const KIND_OPTIONS: {
  value: AlertRuleKind
  label: string
  Icon: typeof GaugeIcon
}[] = [
  { value: 'threshold', label: 'Threshold', Icon: GaugeIcon },
  { value: 'crashloop', label: 'Crashloop', Icon: ArrowCounterClockwiseIcon },
]

const COMPARATOR_OPTIONS: { value: Comparator; label: string }[] = [
  { value: '>', label: '> (greater than)' },
  { value: '<', label: '< (less than)' },
  { value: '>=', label: '>= (greater or equal)' },
  { value: '<=', label: '<= (less or equal)' },
]

// One flat form model covering both rule kinds rather than a
// zod.discriminatedUnion, so every field stays registered regardless of
// the selected kind (matching EnvEditor's plain-object-plus-superRefine
// shape): the kind-specific fields simply aren't rendered or validated
// when the other kind is selected, and onSubmit below picks which of
// them actually go into the request body.
const createAlertRuleSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required'),
    kind: z.enum(['threshold', 'crashloop']),
    metric: z.string().trim(),
    comparator: z.enum(['>', '<', '>=', '<=']),
    threshold: z.coerce.number({ error: 'Must be a number' }),
    forDuration: z.string().trim(),
    restartCountThreshold: z.coerce.number({ error: 'Must be a number' }),
    restartWindow: z.string().trim(),
    channelId: z.string(),
    enabled: z.boolean(),
  })
  .superRefine((data, ctx) => {
    if (data.kind === 'threshold') {
      if (!data.metric) {
        ctx.addIssue({
          code: 'custom',
          message: 'Metric is required',
          path: ['metric'],
        })
      }
      if (data.forDuration && !GO_DURATION_REGEX.test(data.forDuration)) {
        ctx.addIssue({
          code: 'custom',
          message: 'Must look like a duration, e.g. "2m" or "30s"',
          path: ['forDuration'],
        })
      }
      return
    }

    if (
      !Number.isInteger(data.restartCountThreshold) ||
      data.restartCountThreshold <= 0
    ) {
      ctx.addIssue({
        code: 'custom',
        message: 'Must be a positive whole number',
        path: ['restartCountThreshold'],
      })
    }
    if (!data.restartWindow) {
      ctx.addIssue({
        code: 'custom',
        message: 'Restart window is required',
        path: ['restartWindow'],
      })
    } else if (!GO_DURATION_REGEX.test(data.restartWindow)) {
      ctx.addIssue({
        code: 'custom',
        message: 'Must look like a duration, e.g. "5m"',
        path: ['restartWindow'],
      })
    }
  })

// z.coerce.number() has a wider input type (unknown, since it accepts
// whatever a raw <input type="number"> gives it) than output type
// (number), so the form's field-value type (what register/defaultValues
// bind to) and its post-validation submit type have to be declared
// separately: z.input for the former, z.output for the latter. Passing
// only z.infer (the output type) to useForm's generic, which is what
// every other form in this codebase does, does not typecheck here
// because none of those other forms coerce a string input to a number.
type CreateAlertRuleFormInput = z.input<typeof createAlertRuleSchema>
type CreateAlertRuleFormOutput = z.output<typeof createAlertRuleSchema>

const DEFAULT_VALUES: CreateAlertRuleFormInput = {
  name: '',
  kind: 'threshold',
  metric: '',
  comparator: '>',
  threshold: 80,
  forDuration: '2m',
  restartCountThreshold: 5,
  restartWindow: '5m',
  channelId: '',
  enabled: true,
}

// Create-via-dialog flow for a new alert rule (TASKS.md 2.5/2.7),
// following CreateTokenDialog's shape: a trigger button, a form inside
// DialogContent, mutate on submit, close on success. Unlike
// CreateTokenDialog there is no one-time-secret reveal step here, POST
// /api/v1/apps/{name}/alerts's response has nothing sensitive in it, so
// the dialog just closes on success rather than swapping to a second
// view.
export function CreateAlertRuleDialog({ appName }: { appName: string }) {
  const [open, setOpen] = useState(false)
  const createRule = useCreateAlertRule(appName)
  const channelsQuery = useNotificationChannelsOptional()
  const channels = channelsQuery.data ?? []
  const { control, register, handleSubmit, formState, reset, watch } = useForm<
    CreateAlertRuleFormInput,
    unknown,
    CreateAlertRuleFormOutput
  >({
    resolver: zodResolver(createAlertRuleSchema),
    defaultValues: DEFAULT_VALUES,
  })
  const kind = watch('kind')

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(DEFAULT_VALUES)
      createRule.reset()
    }
  }

  const onSubmit = handleSubmit((values) => {
    const req: CreateAlertRuleRequest = {
      name: values.name.trim(),
      kind: values.kind,
      channel_id: values.channelId || undefined,
      enabled: values.enabled,
    }
    if (values.kind === 'threshold') {
      req.metric = values.metric.trim()
      req.comparator = values.comparator
      req.threshold = values.threshold
      req.for_duration = values.forDuration.trim() || undefined
    } else {
      req.restart_count_threshold = values.restartCountThreshold
      req.restart_window = values.restartWindow.trim()
    }
    createRule.mutate(req, {
      onSuccess: () => {
        handleOpenChange(false)
        toast.add({
          title: `Alert rule "${req.name}" created.`,
          type: 'success',
        })
      },
    })
  })

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button size="sm" />}>Create rule</DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5">
            <PlusCircleIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            Create alert rule
          </DialogTitle>
          <DialogDescription>
            A threshold rule watches a metric; a crashloop rule watches
            container restarts. Both notify the same way once they fire.
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          <Field>
            <FieldLabel htmlFor="rule-name">Name</FieldLabel>
            <Input
              id="rule-name"
              placeholder="e.g. high-cpu"
              {...register('name')}
            />
            <FieldError errors={[formState.errors.name]} />
          </Field>

          <Field>
            <FieldLabel htmlFor="rule-kind">Kind</FieldLabel>
            <Controller
              control={control}
              name="kind"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger id="rule-kind" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {KIND_OPTIONS.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        <option.Icon
                          className="size-3.5 text-muted-foreground"
                          aria-hidden="true"
                        />
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </Field>

          {kind === 'threshold' ? (
            <>
              <Field>
                <FieldLabel htmlFor="rule-metric">Metric</FieldLabel>
                <Input
                  id="rule-metric"
                  placeholder="e.g. cpu_percent"
                  {...register('metric')}
                />
                <FieldError errors={[formState.errors.metric]} />
              </Field>

              <div className="flex gap-2">
                <Field className="w-40 shrink-0">
                  <FieldLabel htmlFor="rule-comparator">Comparator</FieldLabel>
                  <Controller
                    control={control}
                    name="comparator"
                    render={({ field }) => (
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        <SelectTrigger id="rule-comparator" className="w-full">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {COMPARATOR_OPTIONS.map((option) => (
                            <SelectItem key={option.value} value={option.value}>
                              {option.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    )}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="rule-threshold">Threshold</FieldLabel>
                  <Input
                    id="rule-threshold"
                    type="number"
                    step="any"
                    {...register('threshold')}
                  />
                  <FieldError errors={[formState.errors.threshold]} />
                </Field>
              </div>

              <Field>
                <FieldLabel htmlFor="rule-for-duration">
                  For duration (optional)
                </FieldLabel>
                <Input
                  id="rule-for-duration"
                  placeholder="2m"
                  {...register('forDuration')}
                />
                <FieldError errors={[formState.errors.forDuration]} />
              </Field>
            </>
          ) : (
            <>
              <Field>
                <FieldLabel htmlFor="rule-restart-count">
                  Restart count threshold
                </FieldLabel>
                <Input
                  id="rule-restart-count"
                  type="number"
                  step="1"
                  min="1"
                  {...register('restartCountThreshold')}
                />
                <FieldError errors={[formState.errors.restartCountThreshold]} />
              </Field>
              <Field>
                <FieldLabel htmlFor="rule-restart-window">
                  Restart window
                </FieldLabel>
                <Input
                  id="rule-restart-window"
                  placeholder="5m"
                  {...register('restartWindow')}
                />
                <FieldError errors={[formState.errors.restartWindow]} />
              </Field>
            </>
          )}

          <Field>
            <FieldLabel htmlFor="rule-channel">
              Notify channel (optional)
            </FieldLabel>
            {channels.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                No channels connected yet. Connect one from Settings &rarr;
                Notification channels first.
              </p>
            ) : (
              <>
                <Controller
                  control={control}
                  name="channelId"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger id="rule-channel" className="w-full">
                        <SelectValue placeholder="Choose a connected channel" />
                      </SelectTrigger>
                      <SelectContent>
                        {channels.map((channel) => (
                          <SelectItem key={channel.id} value={channel.id}>
                            {channel.name} ({CHANNEL_KIND_LABEL[channel.kind]})
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                />
                <FieldDescription>
                  Reuses a channel connection from Settings &rarr;
                  Notification channels; leave unset to create the rule
                  without a notify destination.
                </FieldDescription>
              </>
            )}
          </Field>

          <Field orientation="horizontal">
            <Controller
              control={control}
              name="enabled"
              render={({ field }) => (
                <Switch
                  id="rule-enabled"
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            />
            <FieldLabel htmlFor="rule-enabled">Enabled</FieldLabel>
          </Field>

          {createRule.isError ? (
            <p className="text-sm text-destructive">
              {createRule.error.message}
            </p>
          ) : null}

          <DialogFooter>
            <Button type="submit" disabled={createRule.isPending}>
              {createRule.isPending ? 'Creating...' : 'Create rule'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
