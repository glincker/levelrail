import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PencilSimpleIcon, PlusIcon } from '@phosphor-icons/react/dist/ssr'
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
import {
  useCreateAuditSink,
  useUpdateAuditSink,
} from '../../queries/auditStream'
import type {
  AuditSink,
  AuditSinkConfig,
  AuditSinkKind,
} from '../../types/auditStream'

const KINDS: AuditSinkKind[] = ['syslog', 'webhook', 's3']

function configFor(
  kind: AuditSinkKind,
  f: Required<AuditSinkConfig>,
): AuditSinkConfig {
  switch (kind) {
    case 'syslog':
      return { address: f.address.trim(), tls: f.tls }
    case 'webhook':
      return { url: f.url.trim() }
    case 's3':
      return {
        endpoint: f.endpoint.trim(),
        region: f.region.trim(),
        bucket: f.bucket.trim(),
        prefix: f.prefix.trim(),
        path_style: f.path_style,
      }
  }
}

export function AuditSinkFormDialog({ sink }: { sink?: AuditSink }) {
  const { t } = useTranslation('auditStream')
  const create = useCreateAuditSink()
  const update = useUpdateAuditSink()
  const editing = sink !== undefined
  const mutation = editing ? update : create
  const [open, setOpen] = useState(false)
  const [name, setName] = useState(sink?.name ?? '')
  const [kind, setKind] = useState<AuditSinkKind>(sink?.kind ?? 'syslog')
  const [fields, setFields] = useState<Required<AuditSinkConfig>>({
    address: sink?.config.address ?? '',
    tls: sink?.config.tls ?? true,
    url: sink?.config.url ?? '',
    endpoint: sink?.config.endpoint ?? '',
    region: sink?.config.region ?? '',
    bucket: sink?.config.bucket ?? '',
    prefix: sink?.config.prefix ?? '',
    path_style: sink?.config.path_style ?? false,
  })
  const [accessKey, setAccessKey] = useState('')
  const [secretKey, setSecretKey] = useState('')
  const [backfill, setBackfill] = useState(false)
  const [secret, setSecret] = useState<string | null>(null)

  function setField<K extends keyof AuditSinkConfig>(
    key: K,
    value: Required<AuditSinkConfig>[K],
  ) {
    setFields((prev) => ({ ...prev, [key]: value }))
  }

  function close() {
    setOpen(false)
    setSecret(null)
    mutation.reset()
  }

  function submit(e: React.FormEvent) {
    e.preventDefault()
    const config = configFor(kind, fields)
    if (editing) {
      update.mutate(
        {
          id: sink.id,
          body: {
            name: name.trim(),
            config,
            ...(accessKey ? { access_key_id: accessKey } : {}),
            ...(secretKey ? { secret_access_key: secretKey } : {}),
          },
        },
        { onSuccess: close },
      )
      return
    }
    create.mutate(
      {
        name: name.trim(),
        kind,
        enabled: true,
        config,
        backfill,
        ...(kind === 's3'
          ? { access_key_id: accessKey, secret_access_key: secretKey }
          : {}),
      },
      {
        onSuccess: (created) => {
          if (created.signing_secret) {
            setSecret(created.signing_secret)
          } else {
            close()
          }
        },
      },
    )
  }

  const credentialsMissing =
    !editing && kind === 's3' && (accessKey === '' || secretKey === '')
  const targetMissing =
    (kind === 'syslog' && fields.address.trim() === '') ||
    (kind === 'webhook' && fields.url.trim() === '') ||
    (kind === 's3' && fields.bucket.trim() === '')

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (next) {
          setOpen(true)
        } else {
          close()
        }
      }}
    >
      <DialogTrigger
        render={editing ? <Button variant="outline" size="sm" /> : <Button />}
      >
        {editing ? (
          <PencilSimpleIcon className="size-3.5" aria-hidden="true" />
        ) : (
          <PlusIcon className="size-3.5" aria-hidden="true" />
        )}
        {editing ? t('sinks.card.edit') : t('sinks.add')}
      </DialogTrigger>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {editing
              ? t('sinks.form.editTitle', { name: sink.name })
              : t('sinks.form.createTitle')}
          </DialogTitle>
          <DialogDescription>{t('sinks.form.description')}</DialogDescription>
        </DialogHeader>
        {secret ? (
          <div className="space-y-4">
            <Alert>
              <AlertDescription className="space-y-1">
                <span className="block">{t('sinks.form.secretOnce')}</span>
                <code className="block break-all font-mono text-xs">
                  {secret}
                </code>
              </AlertDescription>
            </Alert>
            <DialogFooter>
              <Button onClick={close}>{t('sinks.form.done')}</Button>
            </DialogFooter>
          </div>
        ) : (
          <form onSubmit={submit} className="space-y-4">
            <Field>
              <FieldLabel htmlFor="sink-name">
                {t('sinks.form.name')}
              </FieldLabel>
              <Input
                id="sink-name"
                value={name}
                placeholder={t('sinks.form.namePlaceholder')}
                onChange={(e) => {
                  setName(e.target.value)
                }}
              />
            </Field>
            <Field>
              <FieldLabel>{t('sinks.form.kind')}</FieldLabel>
              <Select
                value={kind}
                disabled={editing}
                onValueChange={(value) => {
                  if (value) setKind(value)
                }}
              >
                <SelectTrigger aria-label={t('sinks.form.kind')}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {KINDS.map((k) => (
                    <SelectItem key={k} value={k}>
                      {t(`sinks.kind.${k}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            {kind === 'syslog' ? (
              <>
                <Field>
                  <FieldLabel htmlFor="sink-address">
                    {t('sinks.form.address')}
                  </FieldLabel>
                  <Input
                    id="sink-address"
                    value={fields.address}
                    placeholder={t('sinks.form.addressPlaceholder')}
                    onChange={(e) => {
                      setField('address', e.target.value)
                    }}
                  />
                </Field>
                <label className="flex items-center gap-2 text-sm text-foreground">
                  <Switch
                    checked={fields.tls}
                    onCheckedChange={(checked) => {
                      setField('tls', checked)
                    }}
                  />
                  {t('sinks.form.tls')}
                </label>
              </>
            ) : null}
            {kind === 'webhook' ? (
              <Field>
                <FieldLabel htmlFor="sink-url">
                  {t('sinks.form.url')}
                </FieldLabel>
                <Input
                  id="sink-url"
                  value={fields.url}
                  placeholder={t('sinks.form.urlPlaceholder')}
                  onChange={(e) => {
                    setField('url', e.target.value)
                  }}
                />
              </Field>
            ) : null}
            {kind === 's3' ? (
              <>
                <Field>
                  <FieldLabel htmlFor="sink-endpoint">
                    {t('sinks.form.endpoint')}
                  </FieldLabel>
                  <Input
                    id="sink-endpoint"
                    value={fields.endpoint}
                    placeholder={t('sinks.form.endpointPlaceholder')}
                    onChange={(e) => {
                      setField('endpoint', e.target.value)
                    }}
                  />
                </Field>
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                  <Field>
                    <FieldLabel htmlFor="sink-bucket">
                      {t('sinks.form.bucket')}
                    </FieldLabel>
                    <Input
                      id="sink-bucket"
                      value={fields.bucket}
                      onChange={(e) => {
                        setField('bucket', e.target.value)
                      }}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="sink-region">
                      {t('sinks.form.region')}
                    </FieldLabel>
                    <Input
                      id="sink-region"
                      value={fields.region}
                      onChange={(e) => {
                        setField('region', e.target.value)
                      }}
                    />
                  </Field>
                </div>
                <Field>
                  <FieldLabel htmlFor="sink-prefix">
                    {t('sinks.form.prefix')}
                  </FieldLabel>
                  <Input
                    id="sink-prefix"
                    value={fields.prefix}
                    placeholder={t('sinks.form.prefixPlaceholder')}
                    onChange={(e) => {
                      setField('prefix', e.target.value)
                    }}
                  />
                </Field>
                <label className="flex items-center gap-2 text-sm text-foreground">
                  <Switch
                    checked={fields.path_style}
                    onCheckedChange={(checked) => {
                      setField('path_style', checked)
                    }}
                  />
                  {t('sinks.form.pathStyle')}
                </label>
                <Field>
                  <FieldLabel htmlFor="sink-access-key">
                    {t('sinks.form.accessKeyId')}
                  </FieldLabel>
                  <Input
                    id="sink-access-key"
                    autoComplete="off"
                    value={accessKey}
                    onChange={(e) => {
                      setAccessKey(e.target.value)
                    }}
                  />
                </Field>
                <Field>
                  <FieldLabel htmlFor="sink-secret-key">
                    {t('sinks.form.secretAccessKey')}
                  </FieldLabel>
                  <Input
                    id="sink-secret-key"
                    type="password"
                    autoComplete="off"
                    value={secretKey}
                    onChange={(e) => {
                      setSecretKey(e.target.value)
                    }}
                  />
                  {editing ? (
                    <FieldDescription>
                      {t('sinks.form.keepCredentials')}
                    </FieldDescription>
                  ) : null}
                </Field>
              </>
            ) : null}
            {!editing ? (
              <label className="flex items-center gap-2 text-sm text-foreground">
                <Switch checked={backfill} onCheckedChange={setBackfill} />
                {t('sinks.form.backfill')}
              </label>
            ) : null}
            {mutation.isError ? (
              <Alert variant="destructive">
                <AlertDescription>{mutation.error.message}</AlertDescription>
              </Alert>
            ) : null}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={close}>
                {t('sinks.form.cancel')}
              </Button>
              <Button
                type="submit"
                disabled={
                  name.trim() === '' ||
                  targetMissing ||
                  credentialsMissing ||
                  mutation.isPending
                }
              >
                {mutation.isPending
                  ? t('sinks.form.submitting')
                  : editing
                    ? t('sinks.form.submitEdit')
                    : t('sinks.form.submit')}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  )
}
