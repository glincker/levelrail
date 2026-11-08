import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { zodResolver } from '@hookform/resolvers/zod'
import {
  Controller,
  useForm,
  type Control,
  type FieldErrors,
} from 'react-hook-form'
import { CirclePlusGlassIcon } from '@thesvg/ui-react'
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
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from '@/components/ui/field'
import { DurationInput } from '@/components/ui/duration-input'
import { toast } from '@/components/ui/toast'
import { useCreateAlertRule } from '../queries/alerts'
import { useNotificationChannelsOptional } from '../queries/notificationChannels'
import { useScheduledTasks } from '../queries/scheduledTasks'
import { useDatabases } from '../queries/databases'
import { useMonitors } from '../queries/monitors'
import { DatabaseRuleFields } from './DatabaseRuleFields'
import {
  TrafficRuleFields,
  type TrafficRuleFormShape,
} from './TrafficRuleFields'
import {
  PARKED_CERT_DEFAULT_DAYS,
  PARKED_CERT_KIND,
  TRAFFIC_DROP_DEFAULTS,
  TRAFFIC_DROP_KIND,
  isTrafficKind,
} from '@/lib/alertTrafficKinds'
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
import type {} from '../types/alerts'
import { isScheduledTaskKind } from '../types/alerts'
import { SloRuleFields, type SloFormShape } from './SloRuleFields'
import {
  ALERT_COMPARATOR_OPTIONS,
  ALERT_KIND_OPTIONS,
  alertRuleFormSchema,
  buildAlertRuleRequest,
  ALERT_RULE_DEFAULT_VALUES,
  type AlertRuleFormInput,
  type AlertRuleFormOutput,
} from '@/lib/alertRuleForm'
import type { AppVolume } from '../types/appDetail'

// DEFAULT_PATCH_STATUS_THRESHOLD mirrors
// internal/alerting.DefaultPatchStatusThreshold: display-only copy for
// the patch_status description below, since a patch_status rule has no
// per-rule threshold field to show instead (the real, enforced value
// lives engine-wide, set by APP_ALERT_PATCH_STATUS_THRESHOLD).
const DEFAULT_PATCH_STATUS_THRESHOLD = 1

// DEFAULT_NODE_CPU_THRESHOLD_PERCENT/DEFAULT_NODE_MEMORY_THRESHOLD_GIB
// mirror internal/alerting.DefaultNodeCPUThresholdPercent/
// DefaultNodeMemoryThresholdBytes for the node_resource_usage
// description below (the real, enforced values live engine-wide, set by
// APP_ALERT_NODE_CPU_THRESHOLD_PERCENT/
// APP_ALERT_NODE_MEMORY_THRESHOLD_BYTES). Memory is shown in GiB, not
// raw bytes, purely for readability here; the env var itself is bytes.
const DEFAULT_NODE_CPU_THRESHOLD_PERCENT = 80
const DEFAULT_NODE_MEMORY_THRESHOLD_GIB = 4

// Create-via-dialog flow for a new alert rule, following
// CreateTokenDialog's shape: a trigger button, a form inside
// DialogContent, mutate on submit, close on success. Unlike
// CreateTokenDialog there is no one-time-secret reveal step here, POST
// /api/v1/apps/{name}/alerts's response has nothing sensitive in it, so
// the dialog just closes on success rather than swapping to a second
// view.
export function CreateAlertRuleDialog({
  appName,
  volumes,
}: {
  appName: string
  volumes?: AppVolume[]
}) {
  const [open, setOpen] = useState(false)
  const createRule = useCreateAlertRule(appName)
  const channelsQuery = useNotificationChannelsOptional()
  const channels = channelsQuery.data ?? []
  const { t } = useTranslation('observability')
  const { t: tAlert } = useTranslation('alertSetup')
  const scheduledTasksQuery = useScheduledTasks(appName)
  const scheduledTasks = scheduledTasksQuery.data ?? []
  const databasesQuery = useDatabases()
  // null data means the monitors API is off, so the kind is not offered.
  const monitorsEnabled = Boolean(useMonitors().data)
  const databases = databasesQuery.data ?? []
  const { control, register, handleSubmit, formState, reset, watch, setValue } =
    useForm<AlertRuleFormInput, unknown, AlertRuleFormOutput>({
      resolver: zodResolver(alertRuleFormSchema),
      defaultValues: ALERT_RULE_DEFAULT_VALUES,
    })
  const kind = watch('kind')
  const selectedChannelId = watch('channelId')
  const backupResourceKind = watch('backupResourceKind')
  const sloObjective = watch('sloObjective')
  const sloTarget = watch('sloTarget')
  const sloLatencyMs = watch('sloLatencyMs')

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      reset(ALERT_RULE_DEFAULT_VALUES)
      createRule.reset()
    }
  }

  const onSubmit = handleSubmit((values) => {
    const req = buildAlertRuleRequest(values, appName)
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
            <CirclePlusGlassIcon
              className="size-5 text-muted-foreground"
              aria-hidden="true"
            />
            Create alert rule
          </DialogTitle>
          <DialogDescription>
            A threshold rule watches a metric; a crashloop rule watches
            container restarts; an OOM killed rule watches out-of-memory kills;
            a certificate expiry rule watches every certificate on the control
            plane; a node patch status rule watches every node&apos;s pending OS
            security patches; a node disk space rule watches every node&apos;s
            disk usage percentage; a node CPU/memory usage rule watches every
            node&apos;s summed CPU and memory usage; a scheduled task failure
            rule watches one of this app&apos;s scheduled tasks; a domain health
            rule watches every domain configured on this app; a backup missing
            rule watches a database or one of this app&apos;s volumes for a
            scheduled backup that stopped running. All nine notify the same way
            once they fire.
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
                <Select
                  value={field.value}
                  onValueChange={(next) => {
                    field.onChange(next)
                    if (next === 'control_plane_backup_stale') {
                      setValue('forDuration', '72h')
                    }
                    if (next === TRAFFIC_DROP_KIND) {
                      setValue('forDuration', TRAFFIC_DROP_DEFAULTS.window)
                      setValue('threshold', TRAFFIC_DROP_DEFAULTS.percent)
                      setValue(
                        'restartCountThreshold',
                        TRAFFIC_DROP_DEFAULTS.minRequests,
                      )
                    }
                    if (next === PARKED_CERT_KIND)
                      setValue('threshold', PARKED_CERT_DEFAULT_DAYS)
                    if (next === 'database_disk_high') setValue('threshold', 85)
                    if (next === 'database_connections_high')
                      setValue('threshold', 80)
                    if (next === 'database_backup_stale')
                      setValue('threshold', 26)
                  }}
                >
                  <SelectTrigger id="rule-kind" className="w-full">
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
              idPrefix="rule"
              kind={kind}
              appName={appName}
              control={control as unknown as Control<TrafficRuleFormShape>}
              errors={
                formState.errors as unknown as FieldErrors<TrafficRuleFormShape>
              }
              register={register}
            />
          ) : kind === 'cert_expiry' ? (
            <p className="text-sm text-muted-foreground">
              Watches every certificate on the whole control plane, not just
              this app&apos;s own domains, and fires as soon as any of them is
              expiring soon or already expired.
            </p>
          ) : kind === 'patch_status' ? (
            <p className="text-sm text-muted-foreground">
              Watches every node on the whole control plane, not just the ones
              this app happens to run on, and fires as soon as any node has at
              least {DEFAULT_PATCH_STATUS_THRESHOLD} pending OS security patch
              (the control plane&apos;s configured threshold, overridable via
              APP_ALERT_PATCH_STATUS_THRESHOLD).
            </p>
          ) : kind === 'node_disk_space' ? (
            <AlertKindGuidance kind="node_disk_space" />
          ) : kind === 'node_cert_expiring' ? (
            <p className="text-sm text-muted-foreground">
              Watches every node&apos;s agent certificate and fires when one is
              inside the control plane&apos;s warning window
              (APP_NODE_CERT_EXPIRY_WARNING, 21 days by default) or already
              expired. Healthy agents renew well before that, so this means
              renewal is failing or the node needs re-enrolling.
            </p>
          ) : kind === 'node_offline' ? (
            <p className="text-sm text-muted-foreground">
              Watches every node on the whole control plane and fires as soon as
              any node is marked offline (its agent stopped heartbeating). It
              resolves when every node is back online.
            </p>
          ) : kind === 'node_resource_usage' ? (
            <p className="text-sm text-muted-foreground">
              Watches every node&apos;s summed CPU and memory usage across its
              placed containers, on the whole control plane, and fires as soon
              as either crosses its threshold: CPU at{' '}
              {DEFAULT_NODE_CPU_THRESHOLD_PERCENT}% (overridable via
              APP_ALERT_NODE_CPU_THRESHOLD_PERCENT), memory at{' '}
              {DEFAULT_NODE_MEMORY_THRESHOLD_GIB} GiB (overridable via
              APP_ALERT_NODE_MEMORY_THRESHOLD_BYTES). Memory has no honest
              node-capacity percentage to compare against today, so unlike CPU
              it&apos;s an absolute floor, not a proportion.
            </p>
          ) : kind === 'control_plane_backup_stale' ||
            kind === 'log_archive_stale' ? (
            <>
              <p className="text-sm text-muted-foreground">
                {kind === 'log_archive_stale'
                  ? 'Watches every log archive policy and fires when one fails or has not shipped logs successfully within its age limit.'
                  : 'Watches the newest control plane self-backup snapshot and fires when it gets too old. Stays quiet when scheduled control plane backups are disabled (APP_CONTROL_PLANE_BACKUP_INTERVAL=0).'}
              </p>
              <Field>
                <FieldLabel htmlFor="rule-cp-backup-max-age">
                  Maximum age (optional)
                </FieldLabel>
                <Controller
                  control={control}
                  name="forDuration"
                  render={({ field }) => (
                    <DurationInput
                      id="rule-cp-backup-max-age"
                      value={field.value}
                      onChange={field.onChange}
                      onBlur={field.onBlur}
                    />
                  )}
                />
                <FieldDescription>
                  {kind === 'log_archive_stale'
                    ? 'Fire once a policy has gone this long without a successful archive. Leave blank for three intervals (at least 2 hours).'
                    : 'Fire once the newest snapshot is older than this. Leave blank for 3 days.'}
                </FieldDescription>
                <FieldError errors={[formState.errors.forDuration]} />
              </Field>
            </>
          ) : kind === 'domain_health' ? (
            <>
              <p className="text-sm text-muted-foreground">
                Watches every domain currently configured on this app (unlike
                cert_expiry above, scoped to this app&apos;s own domains, not
                the whole control plane) and fires if any of them stops
                resolving to this control plane or starts pointing elsewhere,
                e.g. a CNAME silently repointed away.
              </p>
              <Field>
                <FieldLabel htmlFor="rule-domain-health-for-duration">
                  For duration (optional)
                </FieldLabel>
                <Controller
                  control={control}
                  name="forDuration"
                  render={({ field }) => (
                    <DurationInput
                      id="rule-domain-health-for-duration"
                      value={field.value}
                      onChange={field.onChange}
                      onBlur={field.onBlur}
                    />
                  )}
                />
                <FieldDescription>
                  Require a bad DNS check to persist this long before firing, so
                  one transient lookup blip doesn&apos;t page anyone. Leave
                  blank to fire on the first bad check.
                </FieldDescription>
                <FieldError errors={[formState.errors.forDuration]} />
              </Field>
            </>
          ) : kind === 'slo_burn' ? (
            <SloRuleFields
              idPrefix="rule"
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
                <FieldLabel htmlFor="rule-metric">Metric</FieldLabel>
                <Controller
                  control={control}
                  name="metric"
                  render={({ field }) => (
                    <Select value={field.value} onValueChange={field.onChange}>
                      <SelectTrigger id="rule-metric" className="w-full">
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
                <Controller
                  control={control}
                  name="forDuration"
                  render={({ field }) => (
                    <DurationInput
                      id="rule-for-duration"
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
                <FieldLabel htmlFor="rule-scheduled-task">
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
                          id="rule-scheduled-task"
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
                <FieldLabel htmlFor="rule-scheduled-task-failures">
                  Consecutive failures threshold
                </FieldLabel>
                <Input
                  id="rule-scheduled-task-failures"
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
              idPrefix="rule"
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
              idPrefix="rule"
              control={control as unknown as Control<BackupMissingFormShape>}
              errors={formState.errors}
              backupResourceKind={backupResourceKind}
              databases={databases}
              volumes={volumes}
            />
          ) : kind === 'email_delivery_failing' ? (
            <AlertKindGuidance kind="email_delivery_failing" />
          ) : kind === 'cleanup_failing' ? (
            <AlertKindGuidance kind="cleanup_failing" />
          ) : kind === 'deploy_failed' || kind === 'monitor_down' ? (
            <AlertKindGuidance kind={kind} />
          ) : kind === 'budget_threshold' ? (
            <p className="text-sm text-muted-foreground">
              {t('budgetThreshold.hint')}
            </p>
          ) : kind === 'version_skew' ? (
            <p className="text-sm text-muted-foreground">
              Fires while the running control plane is behind the latest release
              on its configured update channel (Settings &gt; Updates). Resolves
              once you upgrade.
            </p>
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
                <Controller
                  control={control}
                  name="restartWindow"
                  render={({ field }) => (
                    <DurationInput
                      id="rule-restart-window"
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
                  Reuses a channel connection from Settings &rarr; Notification
                  channels; leave unset to create the rule without a notify
                  destination.
                </FieldDescription>
              </>
            )}
            {!selectedChannelId ? (
              <p
                role="status"
                className="text-sm text-amber-700 dark:text-amber-400"
              >
                {t('alerts.noChannelWarning')}
              </p>
            ) : null}
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
