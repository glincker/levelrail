import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import {
  useConnectAppImportSession,
  useCreateAppImportSession,
  type AppImportSource,
} from '../queries/appImport'

interface Props {
  // Set when re-attaching the token to an existing session.
  reconnect?: { id: string; url: string; platform: string }
  onCreated: (id: string) => void
}

export function AppImportConnect({ reconnect, onCreated }: Props) {
  const { t } = useTranslation('migration')
  const create = useCreateAppImportSession()
  const connect = useConnectAppImportSession()
  const [url, setUrl] = useState(reconnect?.url ?? '')
  const [token, setToken] = useState('')
  const [docker, setDocker] = useState(reconnect?.platform === 'docker')
  const [snapshot, setSnapshot] = useState('')
  const [loopback, setLoopback] = useState(false)
  const [priv, setPriv] = useState(false)
  const [insecure, setInsecure] = useState(false)
  const pending = create.isPending || connect.isPending
  const error = create.error ?? connect.error

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    const source: AppImportSource = {
      platform: docker ? 'docker' : (reconnect?.platform ?? 'coolify'),
      url: docker ? 'docker://snapshot' : url.trim(),
      token: docker ? '' : token,
      snapshot: docker ? snapshot : undefined,
      allow_loopback: loopback || undefined,
      allow_private: priv || undefined,
      insecure_tls: insecure || undefined,
    }
    const done = (id: string) => {
      setToken('')
      setSnapshot('')
      onCreated(id)
    }
    if (reconnect) {
      connect.mutate(
        { id: reconnect.id, source },
        { onSuccess: (s) => done(s.id) },
      )
      return
    }
    create.mutate(source, { onSuccess: (s) => done(s.id) })
  }

  return (
    <form onSubmit={onSubmit} className="space-y-4">
      <div className="space-y-1">
        <h3 className="text-base font-semibold">
          {reconnect
            ? t('appImport.connect.reconnectTitle')
            : t('appImport.connect.title')}
        </h3>
        <p className="text-sm text-muted-foreground">
          {t('appImport.connect.description')}
        </p>
      </div>
      {reconnect ? null : (
        <div className="flex gap-2" role="group">
          <Button
            type="button"
            size="sm"
            variant={docker ? 'outline' : 'default'}
            aria-pressed={!docker}
            onClick={() => setDocker(false)}
          >
            {t('appImport.connect.modeApi')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant={docker ? 'default' : 'outline'}
            aria-pressed={docker}
            onClick={() => setDocker(true)}
          >
            {t('appImport.connect.modeDocker')}
          </Button>
        </div>
      )}
      {docker ? (
        <div className="space-y-2">
          <p className="text-sm text-muted-foreground">
            {t('appImport.connect.dockerDescription')}
          </p>
          <code className="block rounded-md bg-muted px-3 py-2 text-sm">
            {t('appImport.connect.dockerCommand')}
          </code>
          <Label htmlFor="appimport-snapshot">
            {t('appImport.connect.snapshot')}
          </Label>
          <Textarea
            id="appimport-snapshot"
            required
            rows={8}
            autoComplete="off"
            spellCheck={false}
            className="font-mono text-xs"
            value={snapshot}
            onChange={(e) => setSnapshot(e.target.value)}
          />
          <Input
            type="file"
            accept="application/json,.json"
            aria-label={t('appImport.connect.snapshotFile')}
            onChange={(e) => {
              void e.target.files?.[0]?.text().then(setSnapshot)
            }}
          />
          <p className="text-xs text-muted-foreground">
            {t('appImport.connect.snapshotHelp')}
          </p>
        </div>
      ) : (
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="appimport-url">{t('appImport.connect.url')}</Label>
            <Input
              id="appimport-url"
              required
              type="url"
              readOnly={!!reconnect}
              placeholder="https://coolify.example.com"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="appimport-token">
              {t('appImport.connect.token')}
            </Label>
            <Input
              id="appimport-token"
              required
              type="password"
              autoComplete="off"
              value={token}
              onChange={(e) => setToken(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">
              {t('appImport.connect.tokenHelp')}
            </p>
          </div>
        </div>
      )}
      <details>
        <summary className="cursor-pointer text-sm text-muted-foreground">
          {t('appImport.connect.network')}
        </summary>
        <div className="mt-2 space-y-2">
          <Label className="flex items-center gap-2 font-normal">
            <Checkbox
              aria-label={t('appImport.connect.loopback')}
              checked={loopback}
              onCheckedChange={(v) => setLoopback(v === true)}
            />
            {t('appImport.connect.loopback')}
          </Label>
          <Label className="flex items-center gap-2 font-normal">
            <Checkbox
              aria-label={t('appImport.connect.private')}
              checked={priv}
              onCheckedChange={(v) => setPriv(v === true)}
            />
            {t('appImport.connect.private')}
          </Label>
          <Label className="flex items-center gap-2 font-normal">
            <Checkbox
              aria-label={t('appImport.connect.insecure')}
              checked={insecure}
              onCheckedChange={(v) => setInsecure(v === true)}
            />
            {t('appImport.connect.insecure')}
          </Label>
          <p className="text-xs text-muted-foreground">
            {t('appImport.connect.networkHelp')}
          </p>
        </div>
      </details>
      <p className="text-xs text-muted-foreground">
        {t('appImport.connect.readOnly')}
      </p>
      {error ? (
        <Alert variant="destructive">
          <AlertTitle>{t('appImport.connect.error')}</AlertTitle>
          <AlertDescription className="break-words whitespace-pre-wrap">
            {error.message}
          </AlertDescription>
        </Alert>
      ) : null}
      <Button type="submit" disabled={pending}>
        {pending
          ? t('appImport.connect.submitting')
          : t('appImport.connect.submit')}
      </Button>
    </form>
  )
}
