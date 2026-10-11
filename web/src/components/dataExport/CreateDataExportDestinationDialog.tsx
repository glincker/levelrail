import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'
import { useCreateDataExportDestination } from '../../queries/dataExport'
import { useStorageDestinationsOptional } from '../../queries/storage'
import type {
  CreateDataExportDestinationRequest,
  DataExportKind,
} from '../../types/dataExport'

const KINDS: DataExportKind[] = [
  'logs_loki',
  'logs_http',
  'logs_syslog',
  'audit_webhook',
  'audit_s3',
  'metrics_otlp',
]

const KIND_KEYS = {
  logs_http: 'kinds.logs_http',
  logs_loki: 'kinds.logs_loki',
  logs_syslog: 'kinds.logs_syslog',
  audit_webhook: 'kinds.audit_webhook',
  audit_s3: 'kinds.audit_s3',
  metrics_otlp: 'kinds.metrics_otlp',
} as const satisfies Record<DataExportKind, string>

const LEVELS = ['any', 'debug', 'info', 'warn', 'error'] as const

const LEVEL_KEYS = {
  any: 'levels.any',
  debug: 'levels.debug',
  info: 'levels.info',
  warn: 'levels.warn',
  error: 'levels.error',
} as const

const isLogKind = (k: DataExportKind) => k.startsWith('logs_')

function parseHeaders(raw: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const line of raw.split('\n')) {
    const idx = line.indexOf(':')
    if (idx <= 0) continue
    const name = line.slice(0, idx).trim()
    const value = line.slice(idx + 1).trim()
    if (name && value) out[name] = value
  }
  return out
}

export function CreateDataExportDestinationDialog() {
  const { t } = useTranslation('dataExport')
  const create = useCreateDataExportDestination()
  const storage = useStorageDestinationsOptional()
  const [open, setOpen] = useState(false)
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

  function reset() {
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
    create.reset()
  }

  function submit(e: React.FormEvent) {
    e.preventDefault()
    const req: CreateDataExportDestinationRequest = {
      name: name.trim(),
      kind,
      enabled: true,
    }
    if (kind !== 'audit_s3') req.endpoint = endpoint.trim()
    if (isLogKind(kind)) {
      const list = apps
        .split(',')
        .map((a) => a.trim())
        .filter(Boolean)
      if (list.length > 0) req.apps = list
      if (level !== 'any') req.min_level = level
    }
    const parsed = parseHeaders(headers)
    if (Object.keys(parsed).length > 0 && kind !== 'logs_syslog') {
      req.headers = parsed
    }
    if (kind === 'audit_webhook' && hmacKey) req.hmac_key = hmacKey
    const options: NonNullable<CreateDataExportDestinationRequest['options']> =
      {}
    if (kind === 'audit_s3') {
      options.storage_target_id = target
      if (prefix.trim()) options.prefix = prefix.trim()
    }
    if (kind === 'metrics_otlp' && interval) {
      options.interval_seconds = Number(interval)
    }
    if ((kind === 'audit_s3' || kind === 'audit_webhook') && backfill) {
      options.backfill = true
    }
    if (Object.keys(options).length > 0) req.options = options

    create.mutate(req, {
      onSuccess: (created) => {
        setOpen(false)
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
    logs_syslog: t('form.endpointSyslog'),
    audit_webhook: t('form.endpointHttp'),
    audit_s3: '',
    metrics_otlp: t('form.endpointOtlp'),
  }[kind]

  const targets = storage.data ?? []
  const canSubmit =
    name.trim() !== '' &&
    (kind === 'audit_s3' ? target !== '' : endpoint !== '')

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) reset()
      }}
    >
      <DialogTrigger render={<Button />}>
        <PlusIcon className="size-3.5" aria-hidden="true" />
        {t('add')}
      </DialogTrigger>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('form.title')}</DialogTitle>
          <DialogDescription>{t('form.description')}</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="space-y-4">
          <Field>
            <FieldLabel htmlFor="export-name">{t('form.name')}</FieldLabel>
            <Input
              id="export-name"
              value={name}
              placeholder={t('form.namePlaceholder')}
              onChange={(e) => {
                setName(e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="export-kind">{t('form.kind')}</FieldLabel>
            <Select
              value={kind}
              onValueChange={(v) => {
                if (typeof v === 'string') setKind(v)
              }}
            >
              <SelectTrigger id="export-kind" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {KINDS.map((k) => (
                  <SelectItem key={k} value={k}>
                    {t(KIND_KEYS[k])}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>

          {kind === 'audit_s3' ? (
            <>
              <Field>
                <FieldLabel htmlFor="export-target">
                  {t('form.storageTarget')}
                </FieldLabel>
                <Select
                  value={target}
                  onValueChange={(v) => {
                    if (typeof v === 'string') setTarget(v)
                  }}
                >
                  <SelectTrigger id="export-target" className="w-full">
                    <SelectValue
                      placeholder={t('form.storageTargetPlaceholder')}
                    />
                  </SelectTrigger>
                  <SelectContent>
                    {targets.map((d) => (
                      <SelectItem key={d.id} value={d.id}>
                        {d.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {targets.length === 0 ? (
                  <FieldDescription>
                    {t('form.storageTargetNone')}
                  </FieldDescription>
                ) : null}
              </Field>
              <Field>
                <FieldLabel htmlFor="export-prefix">
                  {t('form.prefix')}
                </FieldLabel>
                <Input
                  id="export-prefix"
                  value={prefix}
                  placeholder="audit-log"
                  onChange={(e) => {
                    setPrefix(e.target.value)
                  }}
                />
              </Field>
            </>
          ) : (
            <Field>
              <FieldLabel htmlFor="export-endpoint">
                {t('form.endpoint')}
              </FieldLabel>
              <Input
                id="export-endpoint"
                value={endpoint}
                placeholder={endpointPlaceholder}
                onChange={(e) => {
                  setEndpoint(e.target.value)
                }}
              />
            </Field>
          )}

          {isLogKind(kind) ? (
            <>
              <Field>
                <FieldLabel htmlFor="export-apps">{t('form.apps')}</FieldLabel>
                <Input
                  id="export-apps"
                  value={apps}
                  placeholder="web, api"
                  onChange={(e) => {
                    setApps(e.target.value)
                  }}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="export-level">
                  {t('form.minLevel')}
                </FieldLabel>
                <Select
                  value={level}
                  onValueChange={(v) => {
                    if (typeof v === 'string') {
                      setLevel(v)
                    }
                  }}
                >
                  <SelectTrigger id="export-level" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {LEVELS.map((l) => (
                      <SelectItem key={l} value={l}>
                        {t(LEVEL_KEYS[l])}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            </>
          ) : null}

          {kind !== 'logs_syslog' && kind !== 'audit_s3' ? (
            <Field>
              <FieldLabel htmlFor="export-headers">
                {t('form.headers')}
              </FieldLabel>
              <Textarea
                id="export-headers"
                value={headers}
                autoComplete="off"
                rows={3}
                onChange={(e) => {
                  setHeaders(e.target.value)
                }}
              />
              <FieldDescription>{t('form.headersHint')}</FieldDescription>
            </Field>
          ) : null}

          {kind === 'audit_webhook' ? (
            <Field>
              <FieldLabel htmlFor="export-hmac">{t('form.hmacKey')}</FieldLabel>
              <Input
                id="export-hmac"
                type="password"
                autoComplete="off"
                value={hmacKey}
                onChange={(e) => {
                  setHmacKey(e.target.value)
                }}
              />
              <FieldDescription>{t('form.hmacHint')}</FieldDescription>
            </Field>
          ) : null}

          {kind === 'metrics_otlp' ? (
            <Field>
              <FieldLabel htmlFor="export-interval">
                {t('form.interval')}
              </FieldLabel>
              <Input
                id="export-interval"
                type="number"
                min={10}
                max={3600}
                value={interval}
                placeholder="60"
                onChange={(e) => {
                  setInterval(e.target.value)
                }}
              />
            </Field>
          ) : null}

          {kind === 'audit_s3' || kind === 'audit_webhook' ? (
            <label className="flex items-center gap-2 text-sm text-foreground">
              <Switch checked={backfill} onCheckedChange={setBackfill} />
              {t('form.backfill')}
            </label>
          ) : null}

          {create.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{create.error.message}</AlertDescription>
            </Alert>
          ) : null}

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setOpen(false)
              }}
            >
              {t('form.cancel')}
            </Button>
            <Button type="submit" disabled={!canSubmit || create.isPending}>
              {create.isPending ? t('form.submitting') : t('form.submit')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
