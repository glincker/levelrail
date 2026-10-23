import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import {
  useCreateTrafficExportSink,
  useUpdateTrafficExportSink,
} from '../../queries/trafficExport'
import {
  TRAFFIC_EXPORT_FIELDS,
  TRAFFIC_EXPORT_IDENTITY_FIELDS,
  TRAFFIC_EXPORT_KINDS,
  type TrafficExportKind,
  type TrafficExportSink,
  type TrafficExportSinkInput,
} from '../../types/trafficExport'

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  /** Present when editing; absent when adding. */
  sink?: TrafficExportSink
}

/** TrafficExportSheet is the add and edit panel; credentials are write-only and blank keeps the stored one. */
export function TrafficExportSheet({ open, onOpenChange, sink }: Props) {
  const { t } = useTranslation('trafficExport')
  const create = useCreateTrafficExportSink()
  const update = useUpdateTrafficExportSink()
  const editing = sink !== undefined

  const [name, setName] = useState(sink?.name ?? '')
  const [kind, setKind] = useState<TrafficExportKind>(sink?.kind ?? 'posthog')
  const [host, setHost] = useState(sink?.host ?? '')
  const [url, setUrl] = useState('')
  const [token, setToken] = useState('')
  const [authHeader, setAuthHeader] = useState('')
  const [fields, setFields] = useState<string[]>(sink?.fields ?? [])
  const [ipTruncate, setIpTruncate] = useState(sink?.ip_truncate ?? true)
  const [sample, setSample] = useState(String(sink?.sample ?? 1))
  const [enabled, setEnabled] = useState(sink?.enabled ?? true)

  const identityOn = fields.some((f) =>
    (TRAFFIC_EXPORT_IDENTITY_FIELDS as readonly string[]).includes(f),
  )
  const pending = create.isPending || update.isPending
  const error = create.error ?? update.error
  const sampleNum = Number(sample)
  const sampleValid =
    sample.trim() !== '' &&
    Number.isFinite(sampleNum) &&
    sampleNum >= 0 &&
    sampleNum <= 1

  function toggleField(field: string, on: boolean) {
    setFields((cur) =>
      on
        ? [...cur.filter((f) => f !== field), field]
        : cur.filter((f) => f !== field),
    )
  }

  function submit(e: React.FormEvent) {
    e.preventDefault()
    const body: TrafficExportSinkInput = {
      name: name.trim(),
      kind,
      enabled,
      fields,
      ip_truncate: ipTruncate,
      sample: sampleNum,
      ...(kind === 'posthog' ? { host: host.trim() } : {}),
      ...(url.trim() ? { url: url.trim() } : {}),
      ...(token.trim() ? { token: token.trim() } : {}),
      ...(authHeader.trim() ? { auth_header: authHeader.trim() } : {}),
    }
    const done = {
      onSuccess: () => {
        onOpenChange(false)
      },
    }
    if (sink) {
      update.mutate({ id: sink.id, body }, done)
    } else {
      create.mutate(body, done)
    }
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="overflow-y-auto sm:max-w-md">
        <SheetHeader>
          <SheetTitle>
            {editing
              ? t('sheet.editTitle', { name: sink.name })
              : t('sheet.addTitle')}
          </SheetTitle>
          <SheetDescription>{t('sheet.description')}</SheetDescription>
        </SheetHeader>
        <form onSubmit={submit} className="flex flex-col gap-4 px-4 pb-4">
          <Field>
            <FieldLabel htmlFor="tex-name">{t('sheet.name')}</FieldLabel>
            <Input
              id="tex-name"
              value={name}
              placeholder={t('sheet.namePlaceholder')}
              onChange={(e) => {
                setName(e.target.value)
              }}
            />
          </Field>
          <Field>
            <FieldLabel>{t('sheet.kind')}</FieldLabel>
            <div
              className="flex flex-wrap gap-2"
              role="group"
              aria-label={t('sheet.kind')}
            >
              {TRAFFIC_EXPORT_KINDS.map((k) => (
                <Button
                  key={k}
                  type="button"
                  size="sm"
                  variant={kind === k ? 'default' : 'outline'}
                  aria-pressed={kind === k}
                  disabled={editing}
                  onClick={() => {
                    setKind(k)
                  }}
                >
                  {t(`kinds.${k}`)}
                </Button>
              ))}
            </div>
          </Field>
          {kind === 'posthog' ? (
            <>
              <Field>
                <FieldLabel htmlFor="tex-host">{t('sheet.host')}</FieldLabel>
                <Input
                  id="tex-host"
                  value={host}
                  placeholder={t('sheet.hostPlaceholder')}
                  onChange={(e) => {
                    setHost(e.target.value)
                  }}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="tex-token">{t('sheet.token')}</FieldLabel>
                <Input
                  id="tex-token"
                  type="password"
                  autoComplete="off"
                  value={token}
                  onChange={(e) => {
                    setToken(e.target.value)
                  }}
                />
                {sink?.token_set ? (
                  <FieldDescription>{t('sheet.tokenStored')}</FieldDescription>
                ) : null}
              </Field>
            </>
          ) : (
            <>
              <Field>
                <FieldLabel htmlFor="tex-url">{t('sheet.url')}</FieldLabel>
                <Input
                  id="tex-url"
                  type="password"
                  autoComplete="off"
                  value={url}
                  placeholder={t('sheet.urlPlaceholder')}
                  onChange={(e) => {
                    setUrl(e.target.value)
                  }}
                />
                {sink?.url_set ? (
                  <FieldDescription>{t('sheet.urlStored')}</FieldDescription>
                ) : null}
              </Field>
              <Field>
                <FieldLabel htmlFor="tex-auth">
                  {t('sheet.authHeader')}
                </FieldLabel>
                <Input
                  id="tex-auth"
                  type="password"
                  autoComplete="off"
                  value={authHeader}
                  onChange={(e) => {
                    setAuthHeader(e.target.value)
                  }}
                />
                <FieldDescription>{t('sheet.authHeaderHint')}</FieldDescription>
              </Field>
            </>
          )}

          <fieldset className="flex flex-col gap-3">
            <legend className="text-sm font-medium">
              {t('sheet.fieldsTitle')}
            </legend>
            <p className="text-sm text-muted-foreground">
              {t('sheet.fieldsHint')}
            </p>
            {TRAFFIC_EXPORT_FIELDS.map((f) => (
              <label key={f} className="flex items-start gap-2 text-sm">
                <Checkbox
                  checked={fields.includes(f)}
                  onCheckedChange={(checked) => {
                    toggleField(f, checked)
                  }}
                  aria-label={t(`fields.${f}.label`)}
                />
                <span className="flex flex-col">
                  <span className="text-foreground">
                    {t(`fields.${f}.label`)}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {t(`fields.${f}.hint`)}
                  </span>
                </span>
              </label>
            ))}
            {fields.includes('client_ip') ? (
              <label className="flex items-start gap-2 text-sm">
                <Switch
                  checked={ipTruncate}
                  onCheckedChange={setIpTruncate}
                  aria-label={t('sheet.ipTruncate')}
                />
                <span className="flex flex-col">
                  <span className="text-foreground">
                    {t('sheet.ipTruncate')}
                  </span>
                  <span className="text-xs text-muted-foreground">
                    {t('sheet.ipTruncateHint')}
                  </span>
                </span>
              </label>
            ) : null}
          </fieldset>

          {identityOn ? (
            <Banner variant="warning" title={t('disclaimer.title')}>
              {t('disclaimer.body')}
            </Banner>
          ) : null}

          <Field>
            <FieldLabel htmlFor="tex-sample">{t('sheet.sample')}</FieldLabel>
            <Input
              id="tex-sample"
              inputMode="decimal"
              value={sample}
              aria-invalid={!sampleValid}
              onChange={(e) => {
                setSample(e.target.value)
              }}
            />
            <FieldDescription>{t('sheet.sampleHint')}</FieldDescription>
          </Field>
          <label className="flex items-center gap-2 text-sm">
            <Switch checked={enabled} onCheckedChange={setEnabled} />
            {t('sheet.enabled')}
          </label>

          {error ? (
            <p role="alert" className="text-sm text-destructive">
              {t('sheet.saveFailed', { message: error.message })}
            </p>
          ) : null}
          <SheetFooter className="flex-row justify-end gap-2 p-0">
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                onOpenChange(false)
              }}
            >
              {t('sheet.cancel')}
            </Button>
            <Button
              type="submit"
              disabled={pending || name.trim() === '' || !sampleValid}
            >
              {pending ? t('sheet.saving') : t('sheet.save')}
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  )
}
