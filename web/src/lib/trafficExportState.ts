import type {
  TrafficExportSink,
  TrafficExportSinkStatus,
} from '../types/trafficExport'

export type TrafficExportChipState =
  'active' | 'idle' | 'disabled' | 'invalid' | 'failing'

/**
 * chipState folds a stored sink and its live status into one chip. A sink that
 * is active but whose last delivery failed shows as failing, so a broken
 * destination is never green.
 */
export function chipState(
  sink: TrafficExportSink,
  status: TrafficExportSinkStatus | undefined,
): TrafficExportChipState {
  if (!sink.enabled) return 'disabled'
  if (!status) return 'idle'
  if (!status.active) return status.last_error_kind ? 'invalid' : 'idle'
  if (status.last_error_kind) return 'failing'
  return 'active'
}

export function hasStoredCredential(sink: TrafficExportSink): boolean {
  return sink.kind === 'posthog'
    ? Boolean(sink.token_set)
    : Boolean(sink.url_set)
}
