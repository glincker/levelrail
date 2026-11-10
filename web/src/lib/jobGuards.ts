import type {
  ScheduledTask,
  ScheduledTaskConcurrencyPolicy,
  ScheduledTaskRequest,
} from '../types/scheduledTasks'

// The overlap names the guard rails section offers. The server stores skip as
// forbid, so a job saved as skip reads back with concurrency_policy forbid.
export type JobOverlapChoice =
  'allow' | 'skip' | 'queue-one' | 'replace' | 'queue'

export const PRIMARY_OVERLAP_CHOICES: JobOverlapChoice[] = [
  'allow',
  'skip',
  'queue-one',
]

export function overlapFromPolicy(
  policy: ScheduledTaskConcurrencyPolicy | undefined,
): JobOverlapChoice {
  if (policy === 'forbid' || policy === 'skip') return 'skip'
  return policy ?? 'allow'
}

export function policyFromOverlap(
  choice: JobOverlapChoice,
): ScheduledTaskConcurrencyPolicy {
  return choice === 'skip' ? 'forbid' : choice
}

export interface JobGuardsValues {
  overlap: JobOverlapChoice
  timeoutSeconds: string
  alertOnFailure: boolean
  keepRuns: string
  keepDays: string
}

export type JobGuardsField = 'timeoutSeconds' | 'keepRuns' | 'keepDays'

export type JobGuardsPatch = Pick<
  Partial<ScheduledTaskRequest>,
  | 'concurrency_policy'
  | 'timeout_seconds'
  | 'alert_on_failure'
  | 'keep_runs'
  | 'keep_days'
>

function numStr(v: number | undefined): string {
  return v ? String(v) : ''
}

export function guardsFromTask(task: ScheduledTask): JobGuardsValues {
  return {
    overlap: overlapFromPolicy(task.overlap_policy ?? task.concurrency_policy),
    timeoutSeconds: numStr(task.timeout_seconds),
    alertOnFailure: task.alert_on_failure ?? false,
    keepRuns: numStr(task.keep_runs),
    keepDays: numStr(task.keep_days),
  }
}

// Empty means the control plane default, so it is sent as 0.
function countValue(v: string): number {
  return v.trim() === '' ? 0 : Number(v.trim())
}

export function invalidGuardFields(values: JobGuardsValues): JobGuardsField[] {
  const fields: JobGuardsField[] = ['timeoutSeconds', 'keepRuns', 'keepDays']
  return fields.filter((f) => !/^\d*$/.test(values[f].trim()))
}

// Only changed fields go out: the PUT decodes onto the stored job, so an
// omitted field keeps its value and a stale form cannot overwrite it.
export function guardsPatch(
  initial: JobGuardsValues,
  values: JobGuardsValues,
): JobGuardsPatch {
  const patch: JobGuardsPatch = {}
  if (values.overlap !== initial.overlap) {
    patch.concurrency_policy = policyFromOverlap(values.overlap)
  }
  if (
    countValue(values.timeoutSeconds) !== countValue(initial.timeoutSeconds)
  ) {
    patch.timeout_seconds = countValue(values.timeoutSeconds)
  }
  if (values.alertOnFailure !== initial.alertOnFailure) {
    patch.alert_on_failure = values.alertOnFailure
  }
  if (countValue(values.keepRuns) !== countValue(initial.keepRuns)) {
    patch.keep_runs = countValue(values.keepRuns)
  }
  if (countValue(values.keepDays) !== countValue(initial.keepDays)) {
    patch.keep_days = countValue(values.keepDays)
  }
  return patch
}

export type JobGuardsSummaryKey =
  | 'overlap.skip'
  | 'overlap.queue-one'
  | 'overlap.replace'
  | 'overlap.queue'
  | 'timeout'
  | 'alerts'
  | 'keepRuns'
  | 'keepDays'

export interface JobGuardsSummaryPart {
  key: JobGuardsSummaryKey
  seconds?: number
  count?: number
}

// Summary parts as locale keys under guards.summary, skipping defaults so a
// job with no guard rails shows nothing.
export function guardsSummaryParts(
  task: ScheduledTask,
): JobGuardsSummaryPart[] {
  const g = guardsFromTask(task)
  const parts: JobGuardsSummaryPart[] = []
  if (g.overlap !== 'allow') parts.push({ key: `overlap.${g.overlap}` })
  if (task.timeout_seconds) {
    parts.push({ key: 'timeout', seconds: task.timeout_seconds })
  }
  if (g.alertOnFailure) parts.push({ key: 'alerts' })
  if (task.keep_runs) parts.push({ key: 'keepRuns', count: task.keep_runs })
  if (task.keep_days) parts.push({ key: 'keepDays', count: task.keep_days })
  return parts
}
