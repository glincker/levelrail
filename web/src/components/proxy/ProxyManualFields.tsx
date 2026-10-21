import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  manualDefaults,
  toSettings,
  validateManual,
  type ManualErrorKey,
  type ManualFieldName,
  type ManualFields,
} from '../../lib/proxyManualFields'
import type {
  ProxyProviderKind,
  ProxySettings,
} from '../../queries/proxyIntegration'

function Field({
  name,
  value,
  error,
  onChange,
}: {
  name: ManualFieldName
  value: string
  error: ManualErrorKey | undefined
  onChange: (name: ManualFieldName, v: string) => void
}) {
  const { t } = useTranslation('domains')
  const id = useId()
  const hintId = `${id}-hint`
  return (
    <div className="min-w-0 space-y-1">
      <Label htmlFor={id}>{t(`proxyMissing.manual.fields.${name}`)}</Label>
      <Input
        id={id}
        value={value}
        onChange={(e) => onChange(name, e.target.value)}
        aria-invalid={error ? true : undefined}
        aria-describedby={hintId}
        className="font-mono"
      />
      <p
        id={hintId}
        className={
          error ? 'text-xs text-destructive' : 'text-xs text-muted-foreground'
        }
      >
        {error
          ? t(`proxyMissing.manual.errors.${error}`)
          : t(`proxyMissing.manual.hints.${name}`)}
      </p>
    </div>
  )
}

export function ProxyManualFields({
  kind,
  base,
  pending,
  checking,
  onSave,
  onRecheck,
}: {
  kind: ProxyProviderKind
  base: ProxySettings
  pending: boolean
  checking: boolean
  onSave: (settings: ProxySettings) => void
  onRecheck: () => void
}) {
  const { t } = useTranslation('domains')
  const defaults = manualDefaults(kind)
  const [fields, setFields] = useState<ManualFields>({
    container: base.container || defaults.container,
    dynamic_dir: base.dynamic_dir || defaults.dynamic_dir,
    entrypoint_http: base.entrypoint_http || defaults.entrypoint_http,
    entrypoint_https: base.entrypoint_https || defaults.entrypoint_https,
    cert_resolver: base.cert_resolver || defaults.cert_resolver,
    upstream_host: base.upstream_host || defaults.upstream_host,
  })
  const errors = validateManual(kind, fields)
  const names: ManualFieldName[] =
    kind === 'traefik'
      ? [
          'container',
          'dynamic_dir',
          'entrypoint_http',
          'entrypoint_https',
          'cert_resolver',
          'upstream_host',
        ]
      : ['container', 'dynamic_dir', 'upstream_host']
  const invalid = Object.keys(errors).length > 0
  const change = (name: ManualFieldName, v: string) =>
    setFields((f) => ({ ...f, [name]: v }))
  return (
    <div className="space-y-4 rounded-lg border border-border p-4">
      <p className="text-sm text-muted-foreground">
        {t('proxyMissing.manual.body')}
      </p>
      <div className="grid min-w-0 gap-4 sm:grid-cols-2">
        {names.map((n) => (
          <Field
            key={n}
            name={n}
            value={fields[n]}
            error={errors[n]}
            onChange={change}
          />
        ))}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          disabled={invalid || pending}
          onClick={() => onSave(toSettings(kind, fields, base))}
        >
          {t('proxyMissing.manual.save')}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={checking}
          onClick={onRecheck}
        >
          {t('proxyPicker.recheck')}
        </Button>
      </div>
    </div>
  )
}
