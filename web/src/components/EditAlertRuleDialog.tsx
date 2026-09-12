import { useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { Controller, useForm } from 'react-hook-form'
import { z } from 'zod'
import {
  GaugeIcon,
  PencilSimpleIcon,
  ArrowCounterClockwiseIcon,
  ShieldWarningIcon,
  ClockCountdownIcon,
  WrenchIcon,
  HardDriveIcon,
  CpuIcon,
  GlobeIcon,
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
import { useUpdateAlertRule } from '../queries/alerts'
import { useNotificationChannelsOptional } from '../queries/notificationChannels'
import { useScheduledTasks } from '../queries/scheduledTasks'
import { CHANNEL_KIND_LABEL } from './notificationChannelKind'
import type {
  AlertRule,
  AlertRuleKind,
  Comparator,
  CreateAlertRuleRequest,
} from '../types/alerts'

// Same sanity-check regex CreateAlertRuleDialog uses: not a real
// time.Duration parser, just catches an obviously wrong value before a
// round trip.
const GO_DURATION_REGEX = /^-?(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/

const KIND_OPTIONS: {
  value: AlertRuleKind
  label: string
  Icon: typeof GaugeIcon
}[] = [
  { value: 'threshold', label: 'Threshold', Icon: GaugeIcon },
  { value: 'crashloop', label: 'Crashloop', Icon: ArrowCounterClockwiseIcon },
  { value: 'cert_expiry', label: 'Certificate expiry', Icon: ShieldWarningIcon },
  { value: 'patch_status', label: 'Node patch status', Icon: WrenchIcon },
  { value: 'scheduled_task_failure', label: 'Scheduled task failure', Icon: ClockCountdownIcon },
  { value: 'node_disk_space', label: 'Node disk space', Icon: HardDriveIcon },
  { value: 'node_resource_usage', label: 'Node CPU/memory usage', Icon: CpuIcon },
  { value: 'domain_health', label: 'Domain health', Icon: GlobeIcon },
]

const COMPARATOR_OPTIONS: { value: Comparator; label: string }[] = [
  { value: '>', label: '> (greater than)' },
  { value: '<', label: '< (less than)' },
  { value: '>=', label: '>= (greater or equal)' },
  { value: '<=', label: '<= (less or equal)' },
]

// Same flat-form-plus-superRefine shape CreateAlertRuleDialog uses: kept
// identical field-for-field so the two forms validate the same way, the
// only difference being where defaultValues come from (an existing rule
// here, fixed defaults there).
const editAlertRuleSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required'),
    kind: z.enum([
      'threshold',
      'crashloop',
      'cert_expiry',
      'patch_status',
      'scheduled_task_failure',
      'node_disk_space',
      'node_resource_usage',
      'domain_health',
    ]),
    metric: z.string().trim(),
    comparator: z.enum(['>', '<', '>=', '<=']),
    threshold: z.coerce.number({ error: 'Must be a number' }),
    forDuration: z.string().trim(),
    restartCountThreshold: z.coerce.number({ error: 'Must be a number' }),
    restartWindow: z.string().trim(),
    scheduledTaskId: z.string(),
    channelId: z.string(),
    enabled: z.boolean(),
  })
  .superRefine((data, ctx) => {
    if (
      data.kind === 'cert_expiry' ||
      data.kind === 'patch_status' ||
      data.kind === 'node_disk_space' ||
      data.kind === 'node_resource_usage'
    ) {
      return
    }

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

    if (data.kind === 'domain_health') {
      if (data.forDuration && !GO_DURATION_REGEX.test(data.forDuration)) {
        ctx.addIssue({
          code: 'custom',
          message: 'Must look like a duration, e.g. "2m" or "30s"',
          path: ['forDuration'],
        })
      }
      return
    }

    if (data.kind === 'scheduled_task_failure') {
      if (!data.scheduledTaskId) {
        ctx.addIssue({
          code: 'custom',
          message: 'Choose which scheduled task to watch',
          path: ['scheduledTaskId'],
        })
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

type EditAlertRuleFormInput = z.input<typeof editAlertRuleSchema>
type EditAlertRuleFormOutput = z.output<typeof editAlertRuleSchema>

// defaultsFromRule seeds the form from the rule being edited: PUT
// /api/v1/apps/{name}/alerts/{id} is a full replace
// (handleUpdateAlertRule's own doc comment), so every field the operator
// doesn't touch still needs to round-trip through this form unchanged.
function defaultsFromRule(rule: AlertRule): EditAlertRuleFormInput {
  return {
    name: rule.name,
    kind: rule.kind,
    metric: rule.metric ?? '',
    comparator: rule.comparator ?? '>',
    threshold: rule.threshold,
    forDuration: rule.for_duration ?? '',
    restartCountThreshold: rule.restart_count_threshold,
    restartWindow: rule.restart_window ?? '',
    scheduledTaskId: rule.scheduled_task_id ?? '',
    channelId: rule.channel_id ?? '',
    enabled: rule.enabled,
  }
}

// Edit-via-dialog flow for an existing alert rule, mirroring
// CreateAlertRuleDialog's own shape and validation with two differences:
// the form starts pre-filled from rule (defaultsFromRule) instead of
// fixed defaults, and submit calls PUT through useUpdateAlertRule instead
// of POST. This is what turns "delete and recreate to fix a typo'd
// threshold" into a single edit, without losing the rule's id or its
// evaluation state (SaveRule never touches firing/pending_since on an
// update, see internal/alerting/rules.go's own doc comment).
export function EditAlertRuleDialog({
  appName,
  rule,
}: {
  appName: string
  rule: AlertRule
}) {
  const [open, setOpen] = useState(false)
  const updateRule = useUpdateAlertRule(appName)
  const channelsQuery = useNotificationChannelsOptional()
  const channels = channelsQuery.data ?? []
  const scheduledTasksQuery = useScheduledTasks(appName)
  const scheduledTasks = scheduledTasksQuery.data ?? []
  const defaultValues = defaultsFromRule(rule)
  const { control, register, handleSubmit, formState, reset, watch } = useForm<
    EditAlertRuleFormInput,
    unknown,
    EditAlertRuleFormOutput
  >({
    resolver: zodResolver(editAlertRuleSchema),
    defaultValues,
  })
  const kind = watch('kind')

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(defaultValues)
      updateRule.reset()
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
    } else if (values.kind === 'crashloop') {
      req.restart_count_threshold = values.restartCountThreshold
      req.restart_window = values.restartWindow.trim()
    } else if (values.kind === 'scheduled_task_failure') {
      req.scheduled_task_id = values.scheduledTaskId
      req.restart_count_threshold = values.restartCountThreshold
    } else if (values.kind === 'domain_health') {
      req.for_duration = values.forDuration.trim() || undefined
    }
    updateRule.mutate(
      { id: rule.id, req },
      {
        onSuccess: () => {
          handleOpenChange(false)
          toast.add({
            title: `Alert rule "${req.name}" updated.`,
            type: 'success',
          })
        },
      },
    )
  })

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogTrigger render={<Button variant="outline" size="sm" />}>
        <PencilSimpleIcon className="size-3.5" aria-hidden="true" />
        Edit
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-1.5">
            <PencilSimpleIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            Edit alert rule
          </DialogTitle>
          <DialogDescription>
            Fully replaces this rule&apos;s configuration. Its notification
            delivery history stays attached to the same rule id.
          </DialogDescription>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4"
        >
          <Field>
            <FieldLabel htmlFor="edit-rule-name">Name</FieldLabel>
            <Input
              id="edit-rule-name"
              placeholder="e.g. high-cpu"
              {...register('name')}
            />
            <FieldError errors={[formState.errors.name]} />
          </Field>

          <Field>
            <FieldLabel htmlFor="edit-rule-kind">Kind</FieldLabel>
            <Controller
              control={control}
              name="kind"
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger id="edit-rule-kind" className="w-full">
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

          {kind === 'cert_expiry' ||
          kind === 'patch_status' ||
          kind === 'node_disk_space' ||
          kind === 'node_resource_usage' ? (
            <p className="text-sm text-muted-foreground">
              This kind watches every certificate or node on the whole
              control plane platform-wide, needing no metric or threshold
              of its own.
            </p>
          ) : kind === 'domain_health' ? (
            <Field>
              <FieldLabel htmlFor="edit-rule-domain-health-for-duration">
                For duration (optional)
              </FieldLabel>
              <Input
                id="edit-rule-domain-health-for-duration"
                placeholder="e.g. 10m"
                {...register('forDuration')}
              />
              <FieldError errors={[formState.errors.forDuration]} />
            </Field>
          ) : kind === 'threshold' ? (
            <>
              <Field>
                <FieldLabel htmlFor="edit-rule-metric">Metric</FieldLabel>
                <Input
                  id="edit-rule-metric"
                  placeholder="e.g. cpu_percent"
                  {...register('metric')}
                />
                <FieldError errors={[formState.errors.metric]} />
              </Field>

              <div className="flex gap-2">
                <Field className="w-40 shrink-0">
                  <FieldLabel htmlFor="edit-rule-comparator">
                    Comparator
                  </FieldLabel>
                  <Controller
                    control={control}
                    name="comparator"
                    render={({ field }) => (
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        <SelectTrigger id="edit-rule-comparator" className="w-full">
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
                  <FieldLabel htmlFor="edit-rule-threshold">
                    Threshold
                  </FieldLabel>
                  <Input
                    id="edit-rule-threshold"
                    type="number"
                    step="any"
                    {...register('threshold')}
                  />
                  <FieldError errors={[formState.errors.threshold]} />
                </Field>
              </div>

              <Field>
                <FieldLabel htmlFor="edit-rule-for-duration">
                  For duration (optional)
                </FieldLabel>
                <Input
                  id="edit-rule-for-duration"
                  placeholder="2m"
                  {...register('forDuration')}
                />
                <FieldError errors={[formState.errors.forDuration]} />
              </Field>
            </>
          ) : kind === 'scheduled_task_failure' ? (
            <>
              <Field>
                <FieldLabel htmlFor="edit-rule-scheduled-task">
                  Scheduled task
                </FieldLabel>
                {scheduledTasks.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    This app has no scheduled tasks yet. Create one from the
                    Scheduled tasks section first.
                  </p>
                ) : (
                  <Controller
                    control={control}
                    name="scheduledTaskId"
                    render={({ field }) => (
                      <Select value={field.value} onValueChange={field.onChange}>
                        <SelectTrigger id="edit-rule-scheduled-task" className="w-full">
                          <SelectValue placeholder="Choose a scheduled task" />
                        </SelectTrigger>
                        <SelectContent>
                          {scheduledTasks.map((task) => (
                            <SelectItem key={task.id} value={task.id}>
                              {task.command.join(' ')}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    )}
                  />
                )}
                <FieldError errors={[formState.errors.scheduledTaskId]} />
              </Field>
              <Field>
                <FieldLabel htmlFor="edit-rule-scheduled-task-failures">
                  Consecutive failures threshold
                </FieldLabel>
                <Input
                  id="edit-rule-scheduled-task-failures"
                  type="number"
                  step="1"
                  min="1"
                  {...register('restartCountThreshold')}
                />
                <FieldError errors={[formState.errors.restartCountThreshold]} />
              </Field>
            </>
          ) : (
            <>
              <Field>
                <FieldLabel htmlFor="edit-rule-restart-count">
                  Restart count threshold
                </FieldLabel>
                <Input
                  id="edit-rule-restart-count"
                  type="number"
                  step="1"
                  min="1"
                  {...register('restartCountThreshold')}
                />
                <FieldError errors={[formState.errors.restartCountThreshold]} />
              </Field>
              <Field>
                <FieldLabel htmlFor="edit-rule-restart-window">
                  Restart window
                </FieldLabel>
                <Input
                  id="edit-rule-restart-window"
                  placeholder="5m"
                  {...register('restartWindow')}
                />
                <FieldError errors={[formState.errors.restartWindow]} />
              </Field>
            </>
          )}

          <Field>
            <FieldLabel htmlFor="edit-rule-channel">
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
                      <SelectTrigger id="edit-rule-channel" className="w-full">
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
                  Notification channels; leave unset to remove the rule&apos;s
                  notify destination.
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
                  id="edit-rule-enabled"
                  checked={field.value}
                  onCheckedChange={field.onChange}
                />
              )}
            />
            <FieldLabel htmlFor="edit-rule-enabled">Enabled</FieldLabel>
          </Field>

          {updateRule.isError ? (
            <p className="text-sm text-destructive">
              {updateRule.error.message}
            </p>
          ) : null}

          <DialogFooter>
            <Button type="submit" disabled={updateRule.isPending}>
              {updateRule.isPending ? 'Saving...' : 'Save changes'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
