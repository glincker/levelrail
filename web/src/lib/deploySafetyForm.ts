import type {
  DeploySafetyOverrides,
  DeploySafetySettings,
} from '../types/deployGate'

export type SafetyTextField =
  | 'success_threshold'
  | 'stable_window'
  | 'drain_timeout'
  | 'watch_window'
  | 'watch_max_restarts'
  | 'watch_error_rate'
  | 'watch_min_requests'
  | 'watch_latency_ratio'

interface FieldSpec {
  key: SafetyTextField
  numeric: boolean
}

export const GATE_FIELDS: FieldSpec[] = [
  { key: 'success_threshold', numeric: true },
  { key: 'stable_window', numeric: false },
  { key: 'drain_timeout', numeric: false },
]

export const WATCH_FIELDS: FieldSpec[] = [
  { key: 'watch_window', numeric: false },
  { key: 'watch_max_restarts', numeric: true },
  { key: 'watch_error_rate', numeric: true },
  { key: 'watch_min_requests', numeric: true },
  { key: 'watch_latency_ratio', numeric: true },
]

export type SafetyDraft = Record<SafetyTextField, string>

export const EMPTY_OVERRIDES: DeploySafetyOverrides = {
  success_threshold: null,
  stable_window: null,
  drain_timeout: null,
  watch_window: null,
  watch_max_restarts: null,
  watch_error_rate: null,
  watch_min_requests: null,
  auto_rollback: null,
  watch_mode: null,
  watch_latency_ratio: null,
  watch_soft_rollback: null,
}

// The 5xx share is a fraction on the wire and a percentage in the form.
export function toDraft(o: DeploySafetyOverrides): SafetyDraft {
  const text = (v: string | number | null) => (v === null ? '' : String(v))
  return {
    success_threshold: text(o.success_threshold),
    stable_window: text(o.stable_window),
    drain_timeout: text(o.drain_timeout),
    watch_window: text(o.watch_window),
    watch_max_restarts: text(o.watch_max_restarts),
    watch_error_rate:
      o.watch_error_rate === null ? '' : String(o.watch_error_rate * 100),
    watch_min_requests: text(o.watch_min_requests),
    watch_latency_ratio: text(o.watch_latency_ratio),
  }
}

export function defaultText(
  key: SafetyTextField,
  d: DeploySafetySettings,
): string {
  if (key === 'watch_error_rate') return String(d.watch_error_rate * 100)
  return String(d[key])
}

/**
 * toOverrides applies the draft values for `fields` onto `base`, so the gate
 * card and the auto rollback card can each save without clobbering the other.
 * `extras` carries the non-text overrides the calling card owns.
 */
export function toOverrides(
  draft: SafetyDraft,
  base: DeploySafetyOverrides,
  fields: FieldSpec[],
  extras: Partial<DeploySafetyOverrides> = {},
): DeploySafetyOverrides {
  const num = (v: string) => (v.trim() === '' ? null : Number(v))
  const str = (v: string) => (v.trim() === '' ? null : v.trim())
  const rate = num(draft.watch_error_rate)
  const values: DeploySafetyOverrides = {
    ...base,
    success_threshold: num(draft.success_threshold),
    stable_window: str(draft.stable_window),
    drain_timeout: str(draft.drain_timeout),
    watch_window: str(draft.watch_window),
    watch_max_restarts: num(draft.watch_max_restarts),
    watch_error_rate: rate === null ? null : rate / 100,
    watch_min_requests: num(draft.watch_min_requests),
    watch_latency_ratio: num(draft.watch_latency_ratio),
  }
  const out: DeploySafetyOverrides = { ...base, ...extras }
  for (const { key } of fields) {
    Object.assign(out, { [key]: values[key] })
  }
  return out
}
