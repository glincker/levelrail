import type {
  CreateDataExportDestinationRequest,
  DataExportKind,
} from '../types/dataExport'

export interface AuditSinkValues {
  token: string
  index: string
  source: string
  sourceType: string
  facility: string
  appName: string
  gzip: boolean
  insecure: boolean
}

export const EMPTY_AUDIT_SINK: AuditSinkValues = {
  token: '',
  index: '',
  source: '',
  sourceType: '',
  facility: 'authpriv',
  appName: '',
  gzip: false,
  insecure: false,
}

export const isAuditStreamKind = (k: DataExportKind) =>
  k === 'audit_splunk' || k === 'audit_syslog' || k === 'audit_otlp'

/** applyAuditSink copies an audit sink form onto a create request. */
export function applyAuditSink(
  kind: DataExportKind,
  sink: AuditSinkValues,
  backfill: boolean,
  req: CreateDataExportDestinationRequest,
) {
  if (!isAuditStreamKind(kind)) return
  const options = { ...req.options }
  if (backfill) options.backfill = true
  if (sink.insecure) options.insecure_skip_verify = true
  if (sink.gzip && kind !== 'audit_syslog') options.gzip = true
  if (kind === 'audit_splunk') {
    req.api_key = sink.token.trim()
    if (sink.index.trim()) options.index = sink.index.trim()
    if (sink.source.trim()) options.source = sink.source.trim()
    if (sink.sourceType.trim()) options.source_type = sink.sourceType.trim()
  }
  if (kind === 'audit_syslog') {
    options.facility = sink.facility
    if (sink.appName.trim()) options.app_name = sink.appName.trim()
  }
  req.options = options
}
