import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
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
import type { AuditSinkValues } from '../../lib/auditSink'
import type { DataExportKind } from '../../types/dataExport'

const FACILITIES = [
  'authpriv',
  'auth',
  'daemon',
  'local0',
  'local1',
  'local2',
  'local3',
  'local4',
  'local5',
  'local6',
  'local7',
] as const

/** AuditSinkFields is the kind specific part of the Splunk, syslog and OTLP audit sinks. */
export function AuditSinkFields({
  kind,
  values,
  onChange,
}: {
  kind: DataExportKind
  values: AuditSinkValues
  onChange: (next: Partial<AuditSinkValues>) => void
}) {
  const { t } = useTranslation('dataExport')
  return (
    <>
      {kind === 'audit_splunk' ? (
        <>
          <Field>
            <FieldLabel htmlFor="export-hec-token">
              {t('form.hecToken')}
            </FieldLabel>
            <Input
              id="export-hec-token"
              type="password"
              autoComplete="off"
              value={values.token}
              onChange={(e) => {
                onChange({ token: e.target.value })
              }}
            />
            <FieldDescription>{t('form.hecTokenHint')}</FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="export-hec-index">
              {t('form.index')}
            </FieldLabel>
            <Input
              id="export-hec-index"
              value={values.index}
              onChange={(e) => {
                onChange({ index: e.target.value })
              }}
            />
          </Field>
          <div className="grid grid-cols-2 gap-3">
            <Field>
              <FieldLabel htmlFor="export-hec-source">
                {t('form.source')}
              </FieldLabel>
              <Input
                id="export-hec-source"
                value={values.source}
                onChange={(e) => {
                  onChange({ source: e.target.value })
                }}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="export-hec-sourcetype">
                {t('form.sourceType')}
              </FieldLabel>
              <Input
                id="export-hec-sourcetype"
                value={values.sourceType}
                onChange={(e) => {
                  onChange({ sourceType: e.target.value })
                }}
              />
            </Field>
          </div>
        </>
      ) : null}
      {kind === 'audit_syslog' ? (
        <div className="grid grid-cols-2 gap-3">
          <Field>
            <FieldLabel htmlFor="export-facility">
              {t('form.facility')}
            </FieldLabel>
            <Select
              value={values.facility}
              onValueChange={(v) => {
                if (typeof v === 'string') onChange({ facility: v })
              }}
            >
              <SelectTrigger id="export-facility" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {FACILITIES.map((f) => (
                  <SelectItem key={f} value={f}>
                    {f}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel htmlFor="export-app-name">
              {t('form.appName')}
            </FieldLabel>
            <Input
              id="export-app-name"
              value={values.appName}
              onChange={(e) => {
                onChange({ appName: e.target.value })
              }}
            />
          </Field>
        </div>
      ) : null}
      {kind !== 'audit_syslog' ? (
        <label className="flex items-center gap-2 text-sm text-foreground">
          <Switch
            checked={values.gzip}
            onCheckedChange={(gzip) => {
              onChange({ gzip })
            }}
          />
          {t('form.gzip')}
        </label>
      ) : null}
      <div className="space-y-1">
        <label className="flex items-center gap-2 text-sm text-foreground">
          <Switch
            checked={values.insecure}
            onCheckedChange={(insecure) => {
              onChange({ insecure })
            }}
          />
          {t('form.insecure')}
        </label>
        {values.insecure ? (
          <p className="flex items-start gap-1.5 text-xs text-tone-danger">
            <TriangleAlertGlassIcon
              className="mt-0.5 size-6 shrink-0 text-tone-warning"
              aria-hidden="true"
            />
            {t('form.insecureWarning')}
          </p>
        ) : null}
      </div>
      <p className="text-xs text-muted-foreground">
        {t('form.auditStreamHint')}
      </p>
    </>
  )
}
