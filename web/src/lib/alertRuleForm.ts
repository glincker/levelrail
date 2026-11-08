import { z } from 'zod'
import {
  ArchiveGlassIcon,
  CountdownGlassIcon,
  CpuGlassIcon,
  GaugeGlassIcon,
  GlobeGlassIcon,
  MailGlassIcon,
  RocketGlassIcon,
  RotateCcwGlassIcon,
  ServerGlassIcon,
  ShieldAlertGlassIcon,
  WrenchGlassIcon,
} from '@thesvg/ui-react'
import { isDatabaseRuleKind } from '@/lib/databaseRuleKinds'
import {
  PARKED_CERT_KIND,
  TRAFFIC_DROP_KIND,
  domainFromResourceId,
  isTrafficWindowValid,
  trafficResourceId,
} from '@/lib/alertTrafficKinds'
import { sloConfigFromForm } from '../queries/sloPreview'
import { isScheduledTaskKind } from '../types/alerts'
import type {
  AlertRule,
  AlertRuleKind,
  Comparator,
  CreateAlertRuleRequest,
} from '../types/alerts'

// Client-side sanity check only; the server's time.ParseDuration is the
// real validator and its error is still surfaced on submit.
export const GO_DURATION_REGEX = /^-?(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/

export const ALERT_KIND_OPTIONS: {
  value: AlertRuleKind
  label?: string
  Icon: typeof GaugeGlassIcon
}[] = [
  { value: 'threshold', label: 'Threshold', Icon: GaugeGlassIcon },
  { value: 'crashloop', label: 'Crashloop', Icon: RotateCcwGlassIcon },
  { value: 'oom_killed', Icon: CpuGlassIcon },
  {
    value: 'cert_expiry',
    label: 'Certificate expiry',
    Icon: ShieldAlertGlassIcon,
  },
  { value: 'patch_status', label: 'Node patch status', Icon: WrenchGlassIcon },
  {
    value: 'scheduled_task_failure',
    label: 'Job failure',
    Icon: CountdownGlassIcon,
  },
  {
    value: 'scheduled_task_missed',
    label: 'Job missed a run',
    Icon: CountdownGlassIcon,
  },
  {
    value: 'scheduled_task_running_long',
    label: 'Job running too long',
    Icon: CountdownGlassIcon,
  },
  { value: 'node_disk_space', Icon: ServerGlassIcon },
  {
    value: 'node_resource_usage',
    label: 'Node CPU/memory usage',
    Icon: CpuGlassIcon,
  },
  { value: 'domain_health', label: 'Domain health', Icon: GlobeGlassIcon },
  { value: 'backup_missing', label: 'Backup missing', Icon: ArchiveGlassIcon },
  {
    value: 'database_disk_high',
    Icon: ArchiveGlassIcon,
  },
  {
    value: 'database_connections_high',
    Icon: ArchiveGlassIcon,
  },
  {
    value: 'database_down',
    Icon: ArchiveGlassIcon,
  },
  {
    value: 'database_backup_stale',
    Icon: ArchiveGlassIcon,
  },
  { value: 'node_offline', label: 'Node offline', Icon: ServerGlassIcon },
  {
    value: 'node_cert_expiring',
    label: 'Node agent certificate expiring',
    Icon: ServerGlassIcon,
  },
  {
    value: 'control_plane_backup_stale',
    label: 'Control plane backup stale',
    Icon: ArchiveGlassIcon,
  },
  {
    value: 'log_archive_stale',
    label: 'Log archive stale',
    Icon: ArchiveGlassIcon,
  },
  { value: 'slo_burn', label: 'SLO burn rate', Icon: GaugeGlassIcon },
  {
    value: 'version_skew',
    label: 'Version update available',
    Icon: RocketGlassIcon,
  },
  {
    value: 'budget_threshold',
    label: 'Budget threshold',
    Icon: GaugeGlassIcon,
  },
  {
    value: 'email_delivery_failing',
    Icon: MailGlassIcon,
  },
  { value: 'cleanup_failing', Icon: ArchiveGlassIcon },
  { value: 'deploy_failed', Icon: RocketGlassIcon },
  { value: 'monitor_down', Icon: GlobeGlassIcon },
  { value: 'domain_traffic_drop', Icon: GaugeGlassIcon },
  { value: 'parked_domain_cert_expiry', Icon: ShieldAlertGlassIcon },
]

export const ALERT_COMPARATOR_OPTIONS: { value: Comparator; label: string }[] =
  [
    { value: '>', label: '> (greater than)' },
    { value: '<', label: '< (less than)' },
    { value: '>=', label: '>= (greater or equal)' },
    { value: '<=', label: '<= (less or equal)' },
  ]

// One flat form model for every kind: fields stay registered and only the
// selected kind's fields are validated and sent.
export const alertRuleFormSchema = z
  .object({
    name: z.string().trim().min(1, 'Name is required'),
    kind: z.enum([
      'threshold',
      'crashloop',
      'oom_killed',
      'cert_expiry',
      'patch_status',
      'scheduled_task_failure',
      'scheduled_task_missed',
      'scheduled_task_running_long',
      'node_disk_space',
      'node_resource_usage',
      'domain_health',
      'backup_missing',
      'database_disk_high',
      'database_connections_high',
      'database_down',
      'database_backup_stale',
      'node_offline',
      'node_cert_expiring',
      'control_plane_backup_stale',
      'log_archive_stale',
      'slo_burn',
      'version_skew',
      'budget_threshold',
      'email_delivery_failing',
      'cleanup_failing',
      'deploy_failed',
      'monitor_down',
      'domain_traffic_drop',
      'parked_domain_cert_expiry',
    ]),
    metric: z.string().trim(),
    comparator: z.enum(['>', '<', '>=', '<=']),
    threshold: z.coerce.number({ error: 'Must be a number' }),
    forDuration: z.string().trim(),
    restartCountThreshold: z.coerce.number({ error: 'Must be a number' }),
    restartWindow: z.string().trim(),
    trafficDomain: z.string().trim(),
    scheduledTaskId: z.string(),
    backupResourceKind: z.enum(['database', 'volume', '']),
    backupDatabaseName: z.string(),
    backupVolumeName: z.string(),
    sloObjective: z.enum(['availability', 'latency']),
    sloTarget: z.coerce.number({ error: 'Must be a number' }),
    sloLatencyMs: z.coerce.number({ error: 'Must be a number' }),
    channelId: z.string(),
    enabled: z.boolean(),
  })
  .superRefine((data, ctx) => {
    // cert_expiry, patch_status, node_disk_space, and node_resource_usage
    // need none of the threshold/crashloop fields: they watch every
    // certificate, every node's patch status, every node's disk usage, or
    // every node's CPU/memory usage, on the control plane platform-wide,
    // not a metric or a restart count.
    if (
      data.kind === 'cert_expiry' ||
      data.kind === 'patch_status' ||
      data.kind === 'node_disk_space' ||
      data.kind === 'node_resource_usage' ||
      data.kind === 'node_offline' ||
      data.kind === 'node_cert_expiring' ||
      data.kind === 'control_plane_backup_stale' ||
      data.kind === 'log_archive_stale' ||
      data.kind === 'version_skew' ||
      data.kind === 'email_delivery_failing' ||
      data.kind === 'cleanup_failing' ||
      data.kind === 'deploy_failed' ||
      data.kind === 'monitor_down' ||
      data.kind === 'budget_threshold'
    ) {
      return
    }

    if (data.kind === PARKED_CERT_KIND) {
      if (!Number.isInteger(data.threshold) || data.threshold < 1) {
        ctx.addIssue({
          code: 'custom',
          message: 'Must be a positive whole number of days',
          path: ['threshold'],
        })
      }
      return
    }

    if (data.kind === TRAFFIC_DROP_KIND) {
      if (data.forDuration && !isTrafficWindowValid(data.forDuration)) {
        ctx.addIssue({
          code: 'custom',
          message: 'Must be a duration from 0 to 24h, e.g. "1h" or "30m"',
          path: ['forDuration'],
        })
      }
      if (!(data.threshold >= 0 && data.threshold <= 100)) {
        ctx.addIssue({
          code: 'custom',
          message: 'Must be between 0 and 100',
          path: ['threshold'],
        })
      }
      if (
        !Number.isInteger(data.restartCountThreshold) ||
        data.restartCountThreshold < 1
      ) {
        ctx.addIssue({
          code: 'custom',
          message: 'Must be a positive whole number',
          path: ['restartCountThreshold'],
        })
      }
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

    // domain_health needs no required fields either: it watches every
    // domain already configured on this app. for_duration is still
    // optional here, the same debounce threshold's own uses.
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

    if (data.kind === 'slo_burn') {
      if (!(data.sloTarget >= 50 && data.sloTarget < 100)) {
        ctx.addIssue({
          code: 'custom',
          message: 'Must be at least 50 and below 100, e.g. 99.9',
          path: ['sloTarget'],
        })
      }
      if (data.sloObjective === 'latency' && !(data.sloLatencyMs >= 5)) {
        ctx.addIssue({
          code: 'custom',
          message: 'Must be at least 5 ms',
          path: ['sloLatencyMs'],
        })
      }
      return
    }

    if (isScheduledTaskKind(data.kind)) {
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

    if (isDatabaseRuleKind(data.kind)) {
      if (!data.backupDatabaseName) {
        ctx.addIssue({
          code: 'custom',
          message: 'Choose which database to watch',
          path: ['backupDatabaseName'],
        })
      }
      const max = data.kind === 'database_backup_stale' ? 8760 : 100
      if (
        data.kind !== 'database_down' &&
        (data.threshold < 0 || data.threshold > max)
      ) {
        ctx.addIssue({
          code: 'custom',
          message: `Must be between 0 and ${max}`,
          path: ['threshold'],
        })
      }
      return
    }

    if (data.kind === 'backup_missing') {
      if (!data.backupResourceKind) {
        ctx.addIssue({
          code: 'custom',
          message: 'Choose what this rule watches',
          path: ['backupResourceKind'],
        })
      } else if (
        data.backupResourceKind === 'database' &&
        !data.backupDatabaseName
      ) {
        ctx.addIssue({
          code: 'custom',
          message: 'Choose which database to watch',
          path: ['backupDatabaseName'],
        })
      } else if (
        data.backupResourceKind === 'volume' &&
        !data.backupVolumeName
      ) {
        ctx.addIssue({
          code: 'custom',
          message: 'Choose which volume to watch',
          path: ['backupVolumeName'],
        })
      }
      if (data.forDuration && !GO_DURATION_REGEX.test(data.forDuration)) {
        ctx.addIssue({
          code: 'custom',
          message: 'Must look like a duration, e.g. "6h"',
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

// z.coerce.number() has a wider input than output type, so useForm needs
// both generics declared separately.
export type AlertRuleFormInput = z.input<typeof alertRuleFormSchema>
export type AlertRuleFormOutput = z.output<typeof alertRuleFormSchema>

export const ALERT_RULE_DEFAULT_VALUES: AlertRuleFormInput = {
  name: '',
  kind: 'threshold',
  metric: '',
  comparator: '>',
  threshold: 80,
  forDuration: '2m',
  restartCountThreshold: 5,
  restartWindow: '5m',
  trafficDomain: '',
  scheduledTaskId: '',
  backupResourceKind: '',
  backupDatabaseName: '',
  backupVolumeName: '',
  sloObjective: 'availability',
  sloTarget: 99.9,
  sloLatencyMs: 300,
  channelId: '',
  enabled: true,
}

// A PUT replaces the whole rule, so edit callers pass the noise settings
// the form does not edit via extra to keep saving from resetting them.
export function buildAlertRuleRequest(
  values: AlertRuleFormOutput,
  appName: string,
  extra: Partial<CreateAlertRuleRequest> = {},
): CreateAlertRuleRequest {
  const req: CreateAlertRuleRequest = {
    name: values.name.trim(),
    kind: values.kind,
    channel_id: values.channelId || undefined,
    enabled: values.enabled,
    ...extra,
  }
  if (values.kind === TRAFFIC_DROP_KIND) {
    req.resource_id = trafficResourceId(appName, values.trafficDomain)
    req.for_duration = values.forDuration.trim() || undefined
    req.threshold = values.threshold
    req.restart_count_threshold = values.restartCountThreshold
  } else if (values.kind === PARKED_CERT_KIND) {
    req.threshold = values.threshold
  } else if (values.kind === 'threshold') {
    req.metric = values.metric.trim()
    req.comparator = values.comparator
    req.threshold = values.threshold
    req.for_duration = values.forDuration.trim() || undefined
  } else if (values.kind === 'crashloop' || values.kind === 'oom_killed') {
    req.restart_count_threshold = values.restartCountThreshold
    req.restart_window = values.restartWindow.trim()
  } else if (isScheduledTaskKind(values.kind)) {
    req.scheduled_task_id = values.scheduledTaskId
    req.restart_count_threshold = values.restartCountThreshold
  } else if (
    values.kind === 'domain_health' ||
    values.kind === 'control_plane_backup_stale' ||
    values.kind === 'log_archive_stale'
  ) {
    req.for_duration = values.forDuration.trim() || undefined
  } else if (values.kind === 'slo_burn') {
    req.slo = sloConfigFromForm(
      values.sloObjective,
      values.sloTarget,
      values.sloLatencyMs,
    )
  } else if (isDatabaseRuleKind(values.kind)) {
    req.backup_database_name = values.backupDatabaseName
    if (values.kind !== 'database_down') {
      req.threshold = values.threshold
    }
  } else if (values.kind === 'backup_missing') {
    req.backup_resource_kind = values.backupResourceKind || undefined
    req.for_duration = values.forDuration.trim() || undefined
    if (values.backupResourceKind === 'database') {
      req.backup_database_name = values.backupDatabaseName
    } else if (values.backupResourceKind === 'volume') {
      // A volume's owning service is always this same app in this
      // codebase's single-service-per-app model, so backup_service_name
      // is set here rather than exposed as its own form field.
      req.backup_service_name = appName
      req.backup_volume_name = values.backupVolumeName
    }
  }
  return req
}

export function alertRuleToFormValues(rule: AlertRule): AlertRuleFormInput {
  return {
    name: rule.name,
    kind: rule.kind as AlertRuleFormInput['kind'],
    metric: rule.metric ?? '',
    comparator: rule.comparator ?? '>',
    threshold: rule.threshold,
    forDuration: rule.for_duration ?? '',
    restartCountThreshold: rule.restart_count_threshold,
    restartWindow: rule.restart_window ?? '',
    trafficDomain: domainFromResourceId(rule.resource_id),
    scheduledTaskId: rule.scheduled_task_id ?? '',
    backupResourceKind: rule.backup_resource_kind ?? '',
    backupDatabaseName: rule.backup_database_name ?? '',
    backupVolumeName: rule.backup_volume_name ?? '',
    sloObjective: rule.slo?.objective ?? 'availability',
    sloTarget: rule.slo?.target ?? 99.9,
    sloLatencyMs: rule.slo?.latency_ms ?? 300,
    channelId: rule.channel_id ?? '',
    enabled: rule.enabled,
  }
}
