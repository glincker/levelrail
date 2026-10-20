import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from '@/components/ui/toast'
import {
  applyAuditSink,
  EMPTY_AUDIT_SINK,
  type AuditSinkValues,
} from '../../lib/auditSink'
import {
  hasAppFilter,
  isLogKind,
  isMetricsKind,
  isVendorKind,
  LEVELS,
  parseHeaders,
} from '../../lib/dataExportForm'
import { useCreateDataExportDestination } from '../../queries/dataExport'
import type {
  CreateDataExportDestinationRequest,
  DataExportKind,
} from '../../types/dataExport'

export function useDataExportDestinationForm(onCreated: () => void) {
  const { t } = useTranslation('dataExport')
  const create = useCreateDataExportDestination()
  const [name, setName] = useState('')
  const [kind, setKind] = useState<DataExportKind>('logs_loki')
  const [endpoint, setEndpoint] = useState('')
  const [apps, setApps] = useState('')
  const [level, setLevel] = useState<(typeof LEVELS)[number]>('any')
  const [headers, setHeaders] = useState('')
  const [hmacKey, setHmacKey] = useState('')
  const [target, setTarget] = useState('')
  const [prefix, setPrefix] = useState('')
  const [interval, setInterval] = useState('')
  const [backfill, setBackfill] = useState(false)
  const [apiKey, setApiKey] = useState('')
  const [region, setRegion] = useState('us')
  const [site, setSite] = useState('')
  const [dataset, setDataset] = useState('')
  const [username, setUsername] = useState('')
  const [sink, setSink] = useState<AuditSinkValues>(EMPTY_AUDIT_SINK)

  function reset() {
    setSink(EMPTY_AUDIT_SINK)
    setName('')
    setEndpoint('')
    setApps('')
    setLevel('any')
    setHeaders('')
    setHmacKey('')
    setTarget('')
    setPrefix('')
    setInterval('')
    setBackfill(false)
    setApiKey('')
    setUsername('')
    setRegion('us')
    setSite('')
    setDataset('')
    create.reset()
  }

  function submit(e: React.FormEvent) {
    e.preventDefault()
    const req: CreateDataExportDestinationRequest = {
      name: name.trim(),
      kind,
      enabled: true,
    }
    if (kind !== 'audit_s3' && endpoint.trim() !== '') {
      req.endpoint = endpoint.trim()
    }
    if (
      isVendorKind(kind) ||
      (kind === 'metrics_remote_write' && apiKey.trim())
    )
      req.api_key = apiKey.trim()
    if (hasAppFilter(kind)) {
      const list = apps
        .split(',')
        .map((a) => a.trim())
        .filter(Boolean)
      if (list.length > 0) req.apps = list
    }
    if (isLogKind(kind) && level !== 'any') req.min_level = level
    const parsed = parseHeaders(headers)
    if (
      Object.keys(parsed).length > 0 &&
      kind !== 'logs_syslog' &&
      kind !== 'audit_syslog'
    ) {
      req.headers = parsed
    }
    if (kind === 'audit_webhook' && hmacKey) req.hmac_key = hmacKey
    const options: NonNullable<CreateDataExportDestinationRequest['options']> =
      {}
    if (kind === 'audit_s3') {
      options.storage_target_id = target
      if (prefix.trim()) options.prefix = prefix.trim()
    }
    if (kind === 'metrics_remote_write' && username.trim()) {
      options.username = username.trim()
    }
    if (isMetricsKind(kind) && interval) {
      options.interval_seconds = Number(interval)
    }
    if ((kind === 'audit_s3' || kind === 'audit_webhook') && backfill) {
      options.backfill = true
    }
    if (kind === 'logs_newrelic' || kind === 'logs_axiom') {
      options.region = region
    }
    if (kind === 'logs_datadog' && site.trim()) options.site = site.trim()
    if (kind === 'logs_axiom') options.dataset = dataset.trim()
    if (Object.keys(options).length > 0) req.options = options
    applyAuditSink(kind, sink, backfill, req)

    create.mutate(req, {
      onSuccess: (created) => {
        onCreated()
        reset()
        toast.add({
          title: t('form.created', { name: created.name }),
          type: 'success',
        })
      },
    })
  }

  const endpointPlaceholder = {
    logs_http: t('form.endpointHttp'),
    logs_loki: t('form.endpointLoki'),
    logs_otlp: t('form.endpointOtlp'),
    logs_syslog: t('form.endpointSyslog'),
    logs_newrelic: t('form.endpointOverride'),
    logs_datadog: t('form.endpointOverride'),
    logs_axiom: t('form.endpointOverride'),
    logs_betterstack: t('form.endpointOverride'),
    audit_webhook: t('form.endpointHttp'),
    audit_s3: '',
    audit_splunk: t('form.endpointSplunk'),
    audit_syslog: t('form.endpointAuditSyslog'),
    audit_otlp: t('form.endpointOtlp'),
    metrics_otlp: t('form.endpointOtlp'),
    metrics_remote_write: t('form.endpointRemoteWrite'),
  }[kind]

  const canSubmit =
    name.trim() !== '' &&
    (kind === 'audit_s3'
      ? target !== ''
      : kind === 'audit_splunk'
        ? sink.token.trim() !== '' && endpoint !== ''
        : isVendorKind(kind)
          ? apiKey.trim() !== '' &&
            (kind !== 'logs_axiom' || dataset.trim() !== '')
          : endpoint !== '')

  return {
    name,
    setName,
    kind,
    setKind,
    endpoint,
    setEndpoint,
    apps,
    setApps,
    level,
    setLevel,
    headers,
    setHeaders,
    hmacKey,
    setHmacKey,
    target,
    setTarget,
    prefix,
    setPrefix,
    interval,
    setInterval,
    backfill,
    setBackfill,
    apiKey,
    setApiKey,
    region,
    setRegion,
    site,
    setSite,
    dataset,
    setDataset,
    username,
    setUsername,
    sink,
    setSink,
    reset,
    submit,
    endpointPlaceholder,
    canSubmit,
    create,
  }
}
