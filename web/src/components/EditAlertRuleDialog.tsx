import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { zodResolver } from '@hookform/resolvers/zod'
import {
  Controller,
  useForm,
  type Control,
  type FieldErrors,
} from 'react-hook-form'
import { PencilGlassIcon } from '@thesvg/ui-react'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from '@/components/ui/sheet'
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
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { DurationInput } from '@/components/ui/duration-input'
import { toast } from '@/components/ui/toast'
import { useUpdateAlertRule } from '../queries/alerts'
import { useNotificationChannelsOptional } from '../queries/notificationChannels'
import { useScheduledTasks } from '../queries/scheduledTasks'
import { useDatabases } from '../queries/databases'
import { useMonitors } from '../queries/monitors'
import { DatabaseRuleFields } from './DatabaseRuleFields'
import {
  TrafficRuleFields,
  type TrafficRuleFormShape,
} from './TrafficRuleFields'
import { isTrafficKind } from '@/lib/alertTrafficKinds'
import {
  isDatabaseRuleKind,
  type DatabaseRuleFormShape,
} from '@/lib/databaseRuleKinds'
import { AlertKindGuidance } from './AlertKindGuidance'
import { alertKindLabel, isAlertKindCopyKind } from './alertKindCopy'
import { CHANNEL_KIND_LABEL } from './notificationChannelKind'
import { METRIC_NAME_LABEL, METRIC_NAME_OPTIONS } from './metricName'
import {
  BackupMissingFields,
  type BackupMissingFormShape,
} from './BackupMissingFields'
import type { AlertRule } from '../types/alerts'
import { isScheduledTaskKind } from '../types/alerts'
import { SloRuleFields, type SloFormShape } from './SloRuleFields'
import {
  ALERT_COMPARATOR_OPTIONS,
  ALERT_KIND_OPTIONS,
  alertRuleFormSchema,
  alertRuleToFormValues,
  buildAlertRuleRequest,
  type AlertRuleFormInput,
  type AlertRuleFormOutput,
} from '@/lib/alertRuleForm'
import type { AppVolume } from '../types/appDetail'

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
  volumes,
}: {
  appName: string
  rule: AlertRule
  volumes?: AppVolume[]
}) {
  const { t } = useTranslation('observability')
  const { t: tAlert } = useTranslation('alertSetup')
  const [open, setOpen] = useState(false)
  const updateRule = useUpdateAlertRule(appName)
  const channelsQuery = useNotificationChannelsOptional()
  const channels = channelsQuery.data ?? []
  const scheduledTasksQuery = useScheduledTasks(appName)
  const scheduledTasks = scheduledTasksQuery.data ?? []
  const databasesQuery = useDatabases()
  // null data means the monitors API is off, so the kind is not offered.
  const monitorsEnabled = Boolean(useMonitors().data)
  const databases = databasesQuery.data ?? []
  const defaultValues = alertRuleToFormValues(rule)
  const { control, register, handleSubmit, formState, reset, watch } = useForm<
    AlertRuleFormInput,
    unknown,
    AlertRuleFormOutput
  >({
    resolver: zodResolver(alertRuleFormSchema),
    defaultValues,
  })
  const kind = watch('kind')
  const backupResourceKind = watch('backupResourceKind')
  const sloObjective = watch('sloObjective')
  const sloTarget = watch('sloTarget')
  const sloLatencyMs = watch('sloLatencyMs')

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(defaultValues)
      updateRule.reset()
    }
  }

  const onSubmit = handleSubmit((values) => {
    const req = buildAlertRuleRequest(values, appName, {
      severity: rule.severity,
      labels: rule.labels,
      consecutive_failures: rule.consecutive_failures,
      flap_threshold: rule.flap_threshold,
      flap_window: rule.flap_window,
    })
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
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetTrigger render={<Button variant="outline" size="sm" />}>
        <PencilGlassIcon className="size-5" aria-hidden="true" />
        Edit
      </SheetTrigger>
      <SheetContent className="overflow-y-auto sm:max-w-lg">
        <SheetHeader>
          <SheetTitle className="flex items-center gap-1.5">
            <PencilGlassIcon
              className="size-5 text-muted-foreground"
              aria-hidden="true"
            />
            Edit alert rule
          </SheetTitle>
          <SheetDescription>
            Fully replaces this rule&apos;s configuration. Its notification
            delivery history stays attached to the same rule id.
          </SheetDescription>
        </SheetHeader>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-4 px-4 pb-4"
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
                    {ALERT_KIND_OPTIONS.filter(
                      (option) =>
                        option.value !== 'monitor_down' ||
                        monitorsEnabled ||
                        kind === 'monitor_down',
                    ).map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        <option.Icon
                          className="size-4.5 text-muted-foreground"
                          aria-hidden="true"
                        />
                        {isTrafficKind(option.value)
                          ? option.value === 'domain_traffic_drop'
                            ? tAlert(
                                'trafficKinds.kind.domain_traffic_drop.label',
                              )
                            : tAlert(
                                'trafficKinds.kind.parked_domain_cert_expiry.label',
                              )
                          : isAlertKindCopyKind(option.value)
                            ? alertKindLabel(t, option.value)
                            : option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </Field>

          {isTrafficKind(kind) ? (
            <TrafficRuleFields
              idPrefix="edit-rule"
              kind={kind}
              appName={appName}
              control={control as unknown as Control<TrafficRuleFormShape>}
              errors={
                formState.errors as unknown as FieldErrors<TrafficRuleFormShape>
              }
              register={register}
            />
          ) : kind === 'node_disk_space' ? (
            <AlertKindGuidance kind="node_disk_space" />
          ) : kind === 'email_delivery_failing' ? (
            <AlertKindGuidance kind="email_delivery_failing" />
          ) : kind === 'deploy_failed' || kind === 'monitor_down' ? (
            <AlertKindGuidance kind={kind} />
          ) : kind === 'cert_expiry' ||
            kind === 'patch_status' ||
            kind === 'node_resource_usage' ||
            kind === 'node_offline' ||
            kind === 'node_cert_expiring' ||
            kind === 'budget_threshold' ||
            kind === 'version_skew' ? (
            <p className="text-sm text-muted-foreground">
              This kind watches every certificate or node on the whole control
              plane platform-wide, needing no metric or threshold of its own.
            </p>
          ) : kind === 'control_plane_backup_stale' ||
            kind === 'log_archive_stale' ? (
            <Field>
              <FieldLabel htmlFor="edit-rule-cp-backup-max-age">
                Maximum age (optional)
              </FieldLabel>
              <Controller
                control={control}
                name="forDuration"
                render={({ field }) => (
                  <DurationInput
                    id="edit-rule-cp-backup-max-age"
                    value={field.value}
                    onChange={field.onChange}
                    onBlur={field.onBlur}
                  />
                )}
              />
              <FieldError errors={[formState.errors.forDuration]} />
            </Field>
          ) : kind === 'domain_health' ? (
            <Field>
              <FieldLabel htmlFor="edit-rule-domain-health-for-duration">
                For duration (optional)
              </FieldLabel>
              <Controller
                control={control}
                name="forDuration"
                render={({ field }) => (
                  <DurationInput
                    id="edit-rule-domain-health-for-duration"
                    value={field.value}
                    onChange={field.onChange}
                    onBlur={field.onBlur}
                  />
                )}
              />
              <FieldError errors={[formState.errors.forDuration]} />
            </Field>
          ) : kind === 'slo_burn' ? (
            <SloRuleFields
              idPrefix="edit-rule"
              app={appName}
              control={control as unknown as Control<SloFormShape>}
              errors={formState.errors as FieldErrors<SloFormShape>}
              objective={sloObjective}
              target={sloTarget as string | number}
              latencyMs={sloLatencyMs as string | number}
            />
          ) : kind === 'threshold' ? (
            <>
              <Field>
                <FieldLabel htmlFor="edit-rule-metric">Metric</FieldLabel>
                <Controller
                  control={control}
                  name="metric"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger id="edit-rule-metric" className="w-full">
                        <SelectValue placeholder="Choose a metric" />
                      </SelectTrigger>
                      <SelectContent>
                        {METRIC_NAME_OPTIONS.map((option) => (
                          <SelectItem key={option} value={option}>
                            {METRIC_NAME_LABEL[option]}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
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
                        <SelectTrigger
                          id="edit-rule-comparator"
                          className="w-full"
                        >
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {ALERT_COMPARATOR_OPTIONS.map((option) => (
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
                <Controller
                  control={control}
                  name="forDuration"
                  render={({ field }) => (
                    <DurationInput
                      id="edit-rule-for-duration"
                      value={field.value}
                      onChange={field.onChange}
                      onBlur={field.onBlur}
                    />
                  )}
                />
                <FieldError errors={[formState.errors.forDuration]} />
              </Field>
            </>
          ) : isScheduledTaskKind(kind) ? (
            <>
              <Field>
                <FieldLabel htmlFor="edit-rule-scheduled-task">
                  Scheduled task
                </FieldLabel>
                {scheduledTasks.length === 0 ? (
                  <p className="text-sm text-muted-foreground">
                    This app has no scheduled tasks yet. Create one from the
                    Jobs section first.
                  </p>
                ) : (
                  <Controller
                    control={control}
                    name="scheduledTaskId"
                    render={({ field }) => (
                      <Select
                        value={field.value}
                        onValueChange={field.onChange}
                      >
                        <SelectTrigger
                          id="edit-rule-scheduled-task"
                          className="w-full"
                        >
                          <SelectValue placeholder="Choose a scheduled task" />
                        </SelectTrigger>
                        <SelectContent>
                          {scheduledTasks.map((task) => (
                            <SelectItem key={task.id} value={task.id}>
                              {task.name || task.command.join(' ')}
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
          ) : isDatabaseRuleKind(kind) ? (
            <DatabaseRuleFields
              idPrefix="edit-rule"
              kind={kind}
              control={control as unknown as Control<DatabaseRuleFormShape>}
              errors={
                formState.errors as unknown as FieldErrors<DatabaseRuleFormShape>
              }
              register={register}
              databases={databases}
            />
          ) : kind === 'backup_missing' ? (
            <BackupMissingFields
              idPrefix="edit-rule"
              control={control as unknown as Control<BackupMissingFormShape>}
              errors={formState.errors}
              backupResourceKind={backupResourceKind}
              databases={databases}
              volumes={volumes}
            />
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
                <Controller
                  control={control}
                  name="restartWindow"
                  render={({ field }) => (
                    <DurationInput
                      id="edit-rule-restart-window"
                      value={field.value}
                      onChange={field.onChange}
                      onBlur={field.onBlur}
                    />
                  )}
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
                  Reuses a channel connection from Settings &rarr; Notification
                  channels; leave unset to remove the rule&apos;s notify
                  destination.
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

          <SheetFooter>
            <Button type="submit" disabled={updateRule.isPending}>
              {updateRule.isPending ? 'Saving...' : 'Save changes'}
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  )
}
