import { z } from 'zod'
import { fromCron, refineCronFields, toCron } from './cronSchedule'
import type {
  ScheduledTask,
  ScheduledTaskRequest,
} from '../types/scheduledTasks'

const optionalCount = z.string().trim().regex(/^\d*$/, 'whole number')

export const jobSchema = z
  .object({
    name: z
      .string()
      .trim()
      .regex(/^([a-z0-9][a-z0-9_-]{0,62})?$/, 'invalid'),
    command: z.string().trim(),
    mode: z.enum(['fresh', 'exec', 'http']),
    httpMethod: z.enum(['GET', 'POST', 'PUT', 'PATCH', 'DELETE']),
    httpPath: z.string().trim(),
    httpBody: z.string(),
    httpBackground: z.boolean(),
    timezone: z.string().trim().min(1, 'required'),
    frequency: z.enum(['daily', 'weekly', 'custom']),
    time: z.string().trim(),
    weekday: z.string().trim(),
    customCron: z.string().trim(),
    enabled: z.boolean(),
    concurrencyPolicy: z.enum(['allow', 'forbid', 'replace', 'queue']),
    timeoutSeconds: optionalCount,
    maxRetries: optionalCount,
    retryBackoffSeconds: optionalCount,
    catchUp: z.enum(['skip', 'run_once']),
    catchUpLookbackSeconds: optionalCount,
    memoryLimitMb: optionalCount,
    cpuLimitMillis: optionalCount,
    warnAfterSeconds: optionalCount,
  })
  .superRefine((data, ctx) => {
    refineCronFields(data, ctx)
    if (data.mode === 'http') {
      if (!data.httpPath.startsWith('/') || data.httpPath.startsWith('//')) {
        ctx.addIssue({ code: 'custom', message: 'path', path: ['httpPath'] })
      }
    } else if (data.command === '') {
      ctx.addIssue({ code: 'custom', message: 'required', path: ['command'] })
    }
  })

export type JobFormValues = z.infer<typeof jobSchema>

export const DEFAULT_JOB_VALUES: JobFormValues = {
  name: '',
  command: '',
  mode: 'fresh',
  httpMethod: 'POST',
  httpPath: '',
  httpBody: '',
  httpBackground: false,
  timezone: 'UTC',
  enabled: true,
  concurrencyPolicy: 'forbid',
  timeoutSeconds: '',
  maxRetries: '',
  retryBackoffSeconds: '',
  catchUp: 'skip',
  catchUpLookbackSeconds: '',
  memoryLimitMb: '',
  cpuLimitMillis: '',
  warnAfterSeconds: '',
  ...fromCron('0 3 * * *'),
}

// A command made of ["sh", "-c", script] shows as the bare script; any
// other argv shows space-joined, an honest if lossy display.
function commandToDisplay(command: string[]): string {
  if (command.length === 3 && command[0] === 'sh' && command[1] === '-c') {
    return command[2] ?? ''
  }
  return command.join(' ')
}

function displayToCommand(value: string): string[] {
  return value === '' ? [] : ['sh', '-c', value.trim()]
}

function numStr(v: number | undefined): string {
  return v ? String(v) : ''
}

function numOrUndefined(v: string): number | undefined {
  return v === '' ? undefined : Number(v)
}

export function toFormValues(task: ScheduledTask): JobFormValues {
  return {
    name: task.name ?? '',
    command: commandToDisplay(task.command),
    mode: task.mode ?? 'exec',
    httpMethod: task.http?.method ?? 'POST',
    httpPath: task.http?.path ?? '',
    httpBody: task.http?.body ?? '',
    httpBackground: task.http?.background ?? false,
    timezone: task.timezone || 'UTC',
    enabled: task.enabled,
    concurrencyPolicy: task.concurrency_policy,
    timeoutSeconds: numStr(task.timeout_seconds),
    maxRetries: numStr(task.max_retries),
    retryBackoffSeconds: numStr(task.retry_backoff_seconds),
    catchUp: task.catch_up ?? 'skip',
    catchUpLookbackSeconds: numStr(task.catch_up_lookback_seconds),
    memoryLimitMb: numStr(task.memory_limit_mb),
    cpuLimitMillis: numStr(task.cpu_limit_millis),
    warnAfterSeconds: numStr(task.warn_after_seconds),
    ...fromCron(task.schedule),
  }
}

export function toRequest(values: JobFormValues): ScheduledTaskRequest {
  return {
    name: values.name || undefined,
    mode: values.mode,
    command: values.mode === 'http' ? [] : displayToCommand(values.command),
    http:
      values.mode === 'http'
        ? {
            method: values.httpMethod,
            path: values.httpPath,
            body: values.httpBody || undefined,
            background: values.httpBackground,
          }
        : undefined,
    schedule: toCron(values),
    timezone: values.timezone,
    enabled: values.enabled,
    concurrency_policy: values.concurrencyPolicy,
    timeout_seconds: numOrUndefined(values.timeoutSeconds),
    max_retries: numOrUndefined(values.maxRetries),
    retry_backoff_seconds: numOrUndefined(values.retryBackoffSeconds),
    catch_up: values.catchUp,
    catch_up_lookback_seconds: numOrUndefined(values.catchUpLookbackSeconds),
    memory_limit_mb: numOrUndefined(values.memoryLimitMb),
    cpu_limit_millis: numOrUndefined(values.cpuLimitMillis),
    warn_after_seconds: numOrUndefined(values.warnAfterSeconds),
  }
}
