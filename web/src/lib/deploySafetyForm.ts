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

export const SAFETY_FIELDS: { key: SafetyTextField; numeric: boolean }[] = [
  { key: 'success_threshold', numeric: true },
  { key: 'stable_window', numeric: false },
  { key: 'drain_timeout', numeric: false },
  { key: 'watch_window', numeric: false },
  { key: 'watch_max_restarts', numeric: true },
  { key: 'watch_error_rate', numeric: true },
  { key: 'watch_min_requests', numeric: true },
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
  }
}

export function defaultText(
  key: SafetyTextField,
  d: DeploySafetySettings,
): string {
  if (key === 'watch_error_rate') return String(d.watch_error_rate * 100)
  return String(d[key])
}

export function toOverrides(
  draft: SafetyDraft,
  autoRollback: boolean | null,
): DeploySafetyOverrides {
  const num = (v: string) => (v.trim() === '' ? null : Number(v))
  const str = (v: string) => (v.trim() === '' ? null : v.trim())
  const rate = num(draft.watch_error_rate)
  return {
    success_threshold: num(draft.success_threshold),
    stable_window: str(draft.stable_window),
    drain_timeout: str(draft.drain_timeout),
    watch_window: str(draft.watch_window),
    watch_max_restarts: num(draft.watch_max_restarts),
    watch_error_rate: rate === null ? null : rate / 100,
    watch_min_requests: num(draft.watch_min_requests),
    auto_rollback: autoRollback,
  }
}
