import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Label } from '@/components/ui/label'
import { AppImportSSHConnect } from './AppImportSSHConnect'
import { MigrationSourceTest } from './MigrationSourceTest'
import {
  useConnectAppImportSession,
  useCreateAppImportSession,
  type AppImportSource,
} from '../queries/appImport'

type Mode = 'api' | 'docker' | 'compose' | 'dokku' | 'ssh'
const API_PLATFORMS = ['coolify', 'dokploy', 'caprover'] as const
type ApiPlatform = (typeof API_PLATFORMS)[number]

function isApiPlatform(p: string | undefined): p is ApiPlatform {
  return API_PLATFORMS.some((x) => x === p)
}
type FileMode = Exclude<Mode, 'api' | 'ssh'>

const FILE_MODES: readonly string[] = ['docker', 'compose', 'dokku']

function isFileMode(m: string | undefined): m is FileMode {
  return m !== undefined && FILE_MODES.includes(m)
}

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
  const [mode, setMode] = useState<Mode>(
    reconnect?.platform === 'docker-ssh'
      ? 'ssh'
      : isFileMode(reconnect?.platform)
        ? reconnect.platform
        : 'api',
  )
  const [platform, setPlatform] = useState<ApiPlatform>(
    isApiPlatform(reconnect?.platform) ? reconnect.platform : 'coolify',
  )
  const [appName, setAppName] = useState('')
  const [snapshot, setSnapshot] = useState('')
  const [loopback, setLoopback] = useState(false)
  const [priv, setPriv] = useState(false)
  const [insecure, setInsecure] = useState(false)
  const pending = create.isPending || connect.isPending
  const error = create.error ?? connect.error

  function buildSource(): AppImportSource {
    const fileMode = isFileMode(mode) ? mode : null
    return {
      platform: fileMode ?? platform,
      url: fileMode ? `file://${fileMode}` : url.trim(),
      token: fileMode ? '' : token,
      snapshot: fileMode ? snapshot : undefined,
      dokku_app_name: mode === 'dokku' ? appName.trim() : undefined,
      allow_loopback: loopback || undefined,
      allow_private: priv || undefined,
      insecure_tls: insecure || undefined,
    }
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault()
    if (mode === 'ssh') return
    const source = buildSource()
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
            : t('appImport.connect.title', {
                name: t(`appImport.connect.platformName.${platform}`),
              })}
        </h3>
        <p className="text-sm text-muted-foreground">
          {t('appImport.connect.description')}
        </p>
      </div>
      {reconnect ? null : (
        <div className="flex flex-wrap gap-2" role="group">
          {(['api', 'docker', 'compose', 'dokku', 'ssh'] as const).map((m) => (
            <Button
              key={m}
              type="button"
              size="sm"
              variant={mode === m ? 'default' : 'outline'}
              aria-pressed={mode === m}
              onClick={() => setMode(m)}
            >
              {t(`appImport.connect.mode.${m}`)}
            </Button>
          ))}
        </div>
      )}
      {mode === 'ssh' ? (
        <AppImportSSHConnect reconnect={reconnect} onCreated={onCreated} />
      ) : isFileMode(mode) ? (
        <div className="space-y-2">
          <p className="text-sm text-muted-foreground">
            {t(`appImport.connect.fileDescription.${mode}`)}
          </p>
          <code className="block rounded-md bg-muted px-3 py-2 text-sm">
            {t(`appImport.connect.fileCommand.${mode}`)}
          </code>
          {mode === 'dokku' ? (
            <div className="space-y-1.5">
              <Label htmlFor="appimport-appname">
                {t('appImport.connect.appName')}
              </Label>
              <Input
                id="appimport-appname"
                required
                autoComplete="off"
                value={appName}
                onChange={(e) => setAppName(e.target.value)}
              />
            </div>
          ) : null}
          <Label htmlFor="appimport-snapshot">
            {t(`appImport.connect.fileLabel.${mode}`)}
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
        <div className="space-y-3">
          {reconnect ? null : (
            <div
              className="flex flex-wrap gap-2"
              role="group"
              aria-label={t('appImport.connect.platform')}
            >
              {API_PLATFORMS.map((p) => (
                <Button
                  key={p}
                  type="button"
                  size="sm"
                  variant={platform === p ? 'default' : 'outline'}
                  aria-pressed={platform === p}
                  onClick={() => setPlatform(p)}
                >
                  {t(`appImport.connect.platformName.${p}`)}
                </Button>
              ))}
            </div>
          )}
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1.5">
              <Label htmlFor="appimport-url">
                {t('appImport.connect.url', {
                  name: t(`appImport.connect.platformName.${platform}`),
                })}
              </Label>
              <Input
                id="appimport-url"
                required
                type="url"
                readOnly={!!reconnect}
                placeholder={`https://${platform}.example.com`}
                value={url}
                onChange={(e) => setUrl(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="appimport-token">
                {t(`appImport.connect.tokenFor.${platform}`)}
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
        </div>
      )}
      {mode === 'ssh' ? null : (
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
      )}
      {mode === 'ssh' ? null : (
        <p className="text-xs text-muted-foreground">
          {t('appImport.connect.readOnly')}
        </p>
      )}
      {error && mode !== 'ssh' ? (
        <Alert variant="destructive">
          <AlertTitle>{t('appImport.connect.error')}</AlertTitle>
          <AlertDescription className="break-words whitespace-pre-wrap">
            {error.message}
          </AlertDescription>
        </Alert>
      ) : null}
      {mode === 'ssh' ? null : (
        <div className="space-y-3">
          {mode === 'api' ? (
            <MigrationSourceTest
              name={t(`appImport.connect.platformName.${platform}`)}
              build={buildSource}
              disabled={pending || url.trim() === '' || token === ''}
            />
          ) : null}
          <Button type="submit" disabled={pending}>
            {pending
              ? t('appImport.connect.submitting')
              : t('appImport.connect.submit')}
          </Button>
        </div>
      )}
    </form>
  )
}
