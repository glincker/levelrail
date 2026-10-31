import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { HardDriveUploadGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { formatBytes } from '@/lib/modelPreflight'
import { useCanWriteSensitive } from '../../hooks/useCanWriteSensitive'
import {
  MailStorageProbeError,
  saveMailStorage,
  testMailStorage,
  useMailStorage,
  type MailStorage,
  type MailStorageStep,
} from '../../queries/mailBackup'
import { useInvalidateMail } from '../../queries/mailMailboxes'
import { FIELD_CLASS, PanelCard, Reason } from './mailPanelUi'

/** Destination form, prefilled from the server. The secret is never prefilled. */
export function MailStoragePanel({ app }: { app: string }) {
  const q = useMailStorage(app)
  if (!q.data) return null
  // Keyed by the saved bucket so a refetch after save re-seeds the draft.
  return (
    <StorageForm key={q.data.bucket?.bucket ?? q.data.kind} data={q.data} />
  )
}

function StorageForm({ data }: { data: MailStorage }) {
  const { t } = useTranslation('mailServer')
  const canWrite = useCanWriteSensitive()
  const invalidate = useInvalidateMail()
  const [kind, setKind] = useState(data.kind)
  const [endpoint, setEndpoint] = useState(data.bucket?.endpoint ?? '')
  const [region, setRegion] = useState(data.bucket?.region ?? '')
  const [bucket, setBucket] = useState(data.bucket?.bucket ?? '')
  const [accessKey, setAccessKey] = useState('')
  const [secret, setSecret] = useState('')
  const [pathStyle, setPathStyle] = useState(data.bucket?.path_style ?? false)
  const [steps, setSteps] = useState<MailStorageStep[]>(data.steps)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saved, setSaved] = useState(false)
  const why = canWrite ? undefined : t('backup.needsWrite')

  const apply = (r: MailStorage) => {
    setSteps(r.steps)
    setSaved(r.saved)
  }
  const wrap = async (fn: () => Promise<MailStorage>) => {
    setBusy(true)
    setError(null)
    setSaved(false)
    try {
      apply(await fn())
      await invalidate()
    } catch (e) {
      if (e instanceof MailStorageProbeError) apply(e.result)
      else setError(e instanceof Error ? e.message : t('storage.failed'))
    } finally {
      setSecret('')
      setBusy(false)
    }
  }
  const submit = (e: FormEvent) => {
    e.preventDefault()
    void wrap(() =>
      saveMailStorage({
        app: data.app,
        kind,
        endpoint,
        region,
        bucket,
        access_key_id: accessKey,
        secret_access_key: secret,
        path_style: pathStyle,
      }),
    )
  }
  const field = (
    id: string,
    label: string,
    value: string,
    set: (v: string) => void,
    type = 'text',
  ) => (
    <div className="flex flex-col gap-1">
      <Label htmlFor={id} className="text-xs">
        {label}
      </Label>
      <Input
        id={id}
        type={type}
        value={value}
        autoComplete="off"
        className="h-8"
        onChange={(e) => set(e.target.value)}
      />
    </div>
  )

  return (
    <PanelCard
      label={t('storage.title')}
      title={
        <span className="flex items-center gap-1.5">
          <HardDriveUploadGlassIcon size="1.2em" />
          {t('storage.title')}
        </span>
      }
    >
      <form onSubmit={submit} className="space-y-2">
        <div className="flex flex-wrap items-end gap-2">
          <div className="flex flex-col gap-1">
            <Label htmlFor="mail-storage-kind" className="text-xs">
              {t('storage.kind')}
            </Label>
            <select
              id="mail-storage-kind"
              className={FIELD_CLASS}
              value={kind}
              onChange={(e) =>
                setKind(e.target.value === 's3' ? 's3' : 'local')
              }
            >
              <option value="local">{t('storage.local')}</option>
              <option value="s3">{t('storage.s3')}</option>
            </select>
          </div>
          {kind === 'local' ? (
            <p className="pb-1.5 text-xs text-muted-foreground">
              {t('storage.folder')}:{' '}
              <span className="font-mono">{data.folder}</span>
              {' · '}
              {t('storage.cap')}: {formatBytes(data.cap_bytes)}
            </p>
          ) : (
            <>
              {field(
                'mail-s3-endpoint',
                t('storage.endpoint'),
                endpoint,
                setEndpoint,
              )}
              {field('mail-s3-region', t('storage.region'), region, setRegion)}
              {field('mail-s3-bucket', t('storage.bucket'), bucket, setBucket)}
              {field(
                'mail-s3-key',
                t('storage.accessKey'),
                accessKey,
                setAccessKey,
              )}
              <div className="flex flex-col gap-1">
                <Label htmlFor="mail-s3-secret" className="text-xs">
                  {t('storage.secret')}
                </Label>
                <Input
                  id="mail-s3-secret"
                  type="password"
                  value={secret}
                  autoComplete="new-password"
                  title={t('storage.secretHint')}
                  className="h-8"
                  onChange={(e) => setSecret(e.target.value)}
                />
              </div>
              <label className="flex items-center gap-1.5 pb-1.5 text-xs">
                <input
                  type="checkbox"
                  checked={pathStyle}
                  onChange={(e) => setPathStyle(e.target.checked)}
                />
                {t('storage.pathStyle')}
              </label>
            </>
          )}
          <Reason why={why}>
            <Button size="sm" type="submit" disabled={!canWrite || busy}>
              {busy ? t('storage.saving') : t('storage.save')}
            </Button>
          </Reason>
          <Reason why={why}>
            <Button
              size="sm"
              type="button"
              variant="outline"
              disabled={!canWrite || busy}
              onClick={() => {
                void wrap(() => testMailStorage(data.app))
              }}
            >
              {t('storage.recheck')}
            </Button>
          </Reason>
        </div>
      </form>
      {steps.length > 0 && (
        <ul
          aria-label={t('storage.checks')}
          className="flex flex-wrap gap-2 text-xs"
        >
          {steps.map((s) => (
            <li key={s.name} className="flex items-center gap-1">
              <Badge variant={s.ok ? 'success' : 'destructive'}>
                {t(`storage.step.${s.name}`, { defaultValue: s.name })}{' '}
                {s.ok ? t('storage.pass') : t('storage.fail')}
              </Badge>
              {s.error && <span className="text-tone-danger">{s.error}</span>}
            </li>
          ))}
        </ul>
      )}
      {saved && (
        <p className="text-xs text-tone-success">{t('storage.saved')}</p>
      )}
      {!data.bucket_uploads && (
        <p className="text-xs text-muted-foreground">
          {t('storage.localOnly')}
        </p>
      )}
      {error && (
        <p role="alert" className="text-xs text-tone-danger">
          {error}
        </p>
      )}
    </PanelCard>
  )
}
