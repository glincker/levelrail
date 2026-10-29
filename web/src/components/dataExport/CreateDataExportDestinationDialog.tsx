import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon } from '@thesvg/ui-react'
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
import { BrandIcon } from '../BrandIcon'
import { VendorFields } from './VendorFields'
import { AuditSinkFields } from './AuditSinkFields'
import { useDataExportDestinationForm } from './useDataExportDestinationForm'
import { isAuditStreamKind } from '../../lib/auditSink'
import { KIND_MARKS } from '../../lib/dataExportKinds'
import {
  hasAppFilter,
  isLogKind,
  isMetricsKind,
  isVendorKind,
  KIND_KEYS,
  KINDS,
  LEVEL_KEYS,
  LEVELS,
} from '../../lib/dataExportForm'
import { useStorageDestinationsOptional } from '../../queries/storage'

export function CreateDataExportDestinationDialog() {
  const { t } = useTranslation('dataExport')
  const storage = useStorageDestinationsOptional()
  const [open, setOpen] = useState(false)
  const {
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
  } = useDataExportDestinationForm(() => {
    setOpen(false)
  })
  const targets = storage.data ?? []

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) reset()
      }}
    >
      <DialogTrigger render={<Button />}>
        <PlusGlassIcon className="size-5" aria-hidden="true" />
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
                    <span className="flex items-center gap-2">
                      {KIND_MARKS[k] ? (
                        <BrandIcon name={KIND_MARKS[k]} className="size-4" />
                      ) : (
                        <span className="size-4" aria-hidden="true" />
                      )}
                      {t(KIND_KEYS[k])}
                    </span>
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

          {isVendorKind(kind) ? (
            <VendorFields
              kind={kind}
              values={{ apiKey, region, site, dataset }}
              onChange={(next) => {
                if (next.apiKey !== undefined) setApiKey(next.apiKey)
                if (next.region !== undefined) setRegion(next.region)
                if (next.site !== undefined) setSite(next.site)
                if (next.dataset !== undefined) setDataset(next.dataset)
              }}
            />
          ) : null}

          {isAuditStreamKind(kind) ? (
            <AuditSinkFields
              kind={kind}
              values={sink}
              onChange={(next) => {
                setSink((cur) => ({ ...cur, ...next }))
              }}
            />
          ) : null}

          {hasAppFilter(kind) ? (
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
          ) : null}

          {isLogKind(kind) ? (
            <>
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

          {kind !== 'logs_syslog' &&
          kind !== 'audit_syslog' &&
          kind !== 'audit_s3' &&
          kind !== 'audit_splunk' ? (
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

          {kind === 'metrics_remote_write' ? (
            <>
              <Field>
                <FieldLabel htmlFor="export-rw-user">
                  {t('form.username')}
                </FieldLabel>
                <Input
                  id="export-rw-user"
                  autoComplete="off"
                  value={username}
                  onChange={(e) => {
                    setUsername(e.target.value)
                  }}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="export-rw-key">
                  {t('form.remoteWriteKey')}
                </FieldLabel>
                <Input
                  id="export-rw-key"
                  type="password"
                  autoComplete="off"
                  value={apiKey}
                  onChange={(e) => {
                    setApiKey(e.target.value)
                  }}
                />
                <FieldDescription>
                  {t('form.remoteWriteKeyHint')}
                </FieldDescription>
              </Field>
            </>
          ) : null}

          {isMetricsKind(kind) ? (
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

          {kind === 'audit_s3' ||
          kind === 'audit_webhook' ||
          isAuditStreamKind(kind) ? (
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
