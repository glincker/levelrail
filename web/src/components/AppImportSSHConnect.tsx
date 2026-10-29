import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  FingerprintPatternGlassIcon,
  PlugGlassIcon,
  ShieldCheckGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import {
  useConnectAppImportSession,
  useCreateAppImportSession,
} from '../queries/appImport'
import {
  useSSHHostKeys,
  useSSHTest,
  type AppImportSSHLogin,
  type SSHProbeResult,
} from '../queries/appImportSSH'
import { AppImportSSHFix } from './AppImportSSHFix'

interface Props {
  reconnect?: { id: string; url: string }
  onCreated: (id: string) => void
}

function parseSSHURL(url: string | undefined): { host: string; port: string } {
  if (!url?.startsWith('ssh://')) return { host: '', port: '' }
  try {
    const u = new URL(url)
    return { host: u.hostname.replace(/^\[|\]$/g, ''), port: u.port }
  } catch {
    return { host: '', port: '' }
  }
}

export function AppImportSSHConnect({ reconnect, onCreated }: Props) {
  const { t } = useTranslation('migration')
  const initial = parseSSHURL(reconnect?.url)
  const [host, setHost] = useState(initial.host)
  const [port, setPort] = useState(initial.port)
  const [user, setUser] = useState('root')
  const [key, setKey] = useState('')
  const [passphrase, setPassphrase] = useState('')
  const [password, setPassword] = useState('')
  const [socket, setSocket] = useState('')
  const [fingerprint, setFingerprint] = useState('')
  const [confirmed, setConfirmed] = useState(false)
  const [insecure, setInsecure] = useState(false)
  const [priv, setPriv] = useState(false)
  const [loopback, setLoopback] = useState(false)
  const hostKeys = useSSHHostKeys()
  const test = useSSHTest()
  const create = useCreateAppImportSession()
  const connect = useConnectAppImportSession()
  const submitting = create.isPending || connect.isPending
  const submitError = create.error ?? connect.error
  const keys = hostKeys.data?.ok ? (hostKeys.data.host_keys ?? []) : []
  const keyFailure = hostKeys.data && !hostKeys.data.ok ? hostKeys.data : null
  const hasCredential = key.trim() !== '' || password !== ''
  const pinned = insecure || (fingerprint.trim() !== '' && confirmed)
  const canTest = host.trim() !== '' && hasCredential && pinned

  function login(over?: Partial<AppImportSSHLogin>): AppImportSSHLogin {
    return {
      host: host.trim(),
      port: port ? Number(port) : undefined,
      user: user.trim(),
      private_key: key.trim() ? key : undefined,
      passphrase: passphrase || undefined,
      password: password || undefined,
      host_key_fingerprint: insecure ? undefined : fingerprint.trim(),
      insecure_host_key: insecure || undefined,
      socket_path: socket.trim() || undefined,
      ...over,
    }
  }

  const network = {
    allow_private: priv || undefined,
    allow_loopback: loopback || undefined,
  }

  function readHostKey() {
    hostKeys.mutate({
      ssh: {
        host: host.trim(),
        port: port ? Number(port) : undefined,
        user: user.trim(),
      },
      ...network,
    })
  }

  function runTest(over?: Partial<AppImportSSHLogin>) {
    test.mutate({ ssh: login(over), ...network })
  }

  function pick(fp: string) {
    setFingerprint(fp)
    setConfirmed(false)
    test.reset()
  }

  function connectSession() {
    const source = {
      platform: 'docker-ssh',
      url: '',
      token: '',
      ssh: login(),
      ...network,
    }
    const done = (id: string) => {
      setKey('')
      setPassphrase('')
      setPassword('')
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

  function onKeyFile(file: File | undefined) {
    void file?.text().then(setKey)
  }

  const result: SSHProbeResult | undefined = test.data

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">
        {t('appImport.ssh.description')}
      </p>
      <div className="grid gap-3 sm:grid-cols-3">
        <div className="space-y-1.5 sm:col-span-2">
          <Label htmlFor="appimport-ssh-host">{t('appImport.ssh.host')}</Label>
          <Input
            id="appimport-ssh-host"
            autoComplete="off"
            spellCheck={false}
            readOnly={!!reconnect && initial.host !== ''}
            placeholder="203.0.113.10"
            value={host}
            onChange={(e) => {
              setHost(e.target.value)
              setConfirmed(false)
            }}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="appimport-ssh-port">{t('appImport.ssh.port')}</Label>
          <Input
            id="appimport-ssh-port"
            inputMode="numeric"
            placeholder="22"
            value={port}
            onChange={(e) => setPort(e.target.value.replace(/\D/g, ''))}
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="appimport-ssh-user">{t('appImport.ssh.user')}</Label>
          <Input
            id="appimport-ssh-user"
            autoComplete="off"
            value={user}
            onChange={(e) => setUser(e.target.value)}
          />
        </div>
        <div className="space-y-1.5 sm:col-span-2">
          <Label htmlFor="appimport-ssh-pass">
            {t('appImport.ssh.passphrase')}
          </Label>
          <Input
            id="appimport-ssh-pass"
            type="password"
            autoComplete="off"
            value={passphrase}
            onChange={(e) => setPassphrase(e.target.value)}
          />
        </div>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="appimport-ssh-key">{t('appImport.ssh.key')}</Label>
        <Textarea
          id="appimport-ssh-key"
          className="font-mono text-xs"
          rows={4}
          autoComplete="off"
          spellCheck={false}
          placeholder={t('appImport.ssh.keyPlaceholder')}
          value={key}
          onChange={(e) => setKey(e.target.value)}
        />
        <Input
          type="file"
          aria-label={t('appImport.ssh.keyFile')}
          onChange={(e) => onKeyFile(e.target.files?.[0])}
        />
        <p className="text-xs text-muted-foreground">
          {t('appImport.ssh.keyHelp')}
        </p>
      </div>
      <details>
        <summary className="cursor-pointer text-sm text-muted-foreground">
          {t('appImport.ssh.advanced')}
        </summary>
        <div className="mt-2 space-y-3">
          <div className="space-y-1.5">
            <Label htmlFor="appimport-ssh-password">
              {t('appImport.ssh.password')}
            </Label>
            <Input
              id="appimport-ssh-password"
              type="password"
              autoComplete="off"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="appimport-ssh-socket">
              {t('appImport.ssh.socket')}
            </Label>
            <Input
              id="appimport-ssh-socket"
              autoComplete="off"
              spellCheck={false}
              placeholder="/var/run/docker.sock"
              value={socket}
              onChange={(e) => setSocket(e.target.value)}
            />
          </div>
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
              aria-label={t('appImport.connect.loopback')}
              checked={loopback}
              onCheckedChange={(v) => setLoopback(v === true)}
            />
            {t('appImport.connect.loopback')}
          </Label>
        </div>
      </details>

      <section className="space-y-3 rounded-md border p-3">
        <div className="flex flex-wrap items-center gap-2">
          <FingerprintPatternGlassIcon size="1.2em" aria-hidden />
          <h4 className="text-sm font-medium">
            {t('appImport.ssh.hostKey.title')}
          </h4>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={host.trim() === '' || hostKeys.isPending}
            onClick={readHostKey}
          >
            {hostKeys.isPending
              ? t('appImport.ssh.hostKey.reading')
              : t('appImport.ssh.hostKey.read')}
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">
          {t('appImport.ssh.hostKey.help')}
        </p>
        {keys.length > 0 ? (
          <ul
            className="space-y-1"
            aria-label={t('appImport.ssh.hostKey.list')}
          >
            {keys.map((k) => (
              <li key={k.fingerprint}>
                <Button
                  type="button"
                  size="sm"
                  variant={
                    fingerprint === k.fingerprint ? 'default' : 'outline'
                  }
                  aria-pressed={fingerprint === k.fingerprint}
                  onClick={() => pick(k.fingerprint)}
                >
                  <span className="font-mono text-xs">
                    {k.type} {k.fingerprint}
                  </span>
                </Button>
              </li>
            ))}
          </ul>
        ) : null}
        {keyFailure ? (
          <AppImportSSHFix
            result={keyFailure}
            busy={hostKeys.isPending}
            currentSocket={socket}
            onRecheck={readHostKey}
            onUseFingerprint={pick}
            onUseSocket={setSocket}
            onAllowPrivate={() => setPriv(true)}
          />
        ) : null}
        {hostKeys.isError ? (
          <Alert variant="destructive">
            <AlertDescription>{hostKeys.error.message}</AlertDescription>
          </Alert>
        ) : null}
        <div className="space-y-1.5">
          <Label htmlFor="appimport-ssh-fp">
            {t('appImport.ssh.hostKey.fingerprint')}
          </Label>
          <Input
            id="appimport-ssh-fp"
            className="font-mono text-xs"
            autoComplete="off"
            spellCheck={false}
            disabled={insecure}
            placeholder={t('appImport.ssh.hostKey.placeholder')}
            value={fingerprint}
            onChange={(e) => pick(e.target.value)}
          />
        </div>
        <Label className="flex items-center gap-2 font-normal">
          <Checkbox
            aria-label={t('appImport.ssh.hostKey.confirm')}
            checked={confirmed}
            disabled={insecure || fingerprint.trim() === ''}
            onCheckedChange={(v) => setConfirmed(v === true)}
          />
          {t('appImport.ssh.hostKey.confirm')}
        </Label>
        <details>
          <summary className="cursor-pointer text-xs text-muted-foreground">
            {t('appImport.ssh.hostKey.compare')}
          </summary>
          <code className="mt-1 block rounded-md bg-muted px-2 py-1 text-xs">
            {t('appImport.ssh.hostKey.compareCommand')}
          </code>
        </details>
        <Label className="flex items-center gap-2 font-normal">
          <Checkbox
            aria-label={t('appImport.ssh.hostKey.insecure')}
            checked={insecure}
            onCheckedChange={(v) => setInsecure(v === true)}
          />
          {t('appImport.ssh.hostKey.insecure')}
        </Label>
        {insecure ? (
          <Alert variant="destructive" role="alert">
            <AlertTitle>{t('appImport.ssh.hostKey.insecureTitle')}</AlertTitle>
            <AlertDescription>
              {t('appImport.ssh.hostKey.insecureBody')}
            </AlertDescription>
          </Alert>
        ) : null}
      </section>

      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          variant="outline"
          disabled={!canTest || test.isPending}
          onClick={() => runTest()}
        >
          <PlugGlassIcon size="1.2em" aria-hidden />
          {test.isPending
            ? t('appImport.ssh.testing')
            : t('appImport.ssh.test')}
        </Button>
        <span className="text-xs text-muted-foreground">
          {t('appImport.ssh.readOnlyNote')}
        </span>
      </div>
      {test.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{test.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {result && !result.ok ? (
        <AppImportSSHFix
          result={result}
          busy={test.isPending}
          currentSocket={socket}
          onRecheck={() => runTest()}
          onUseFingerprint={pick}
          onUseSocket={(p) => {
            setSocket(p)
            runTest({ socket_path: p })
          }}
          onAllowPrivate={() => setPriv(true)}
        />
      ) : null}
      {result?.ok && result.overview ? (
        <Alert role="status">
          <ShieldCheckGlassIcon
            className="text-tone-success"
            size="1.2em"
            aria-hidden
          />
          <AlertTitle>{t('appImport.ssh.ok.title')}</AlertTitle>
          <AlertDescription className="space-y-1">
            <p>{result.reads ?? t('appImport.ssh.ok.reads')}</p>
            <p>
              {t('appImport.ssh.ok.counts', {
                version: result.overview.docker_version,
                containers: result.overview.containers,
                running: result.overview.running,
                volumes: result.overview.volumes,
                images: result.overview.images,
              })}
            </p>
            {(result.warnings ?? []).map((w) => (
              <p key={w} className="text-destructive">
                {w}
              </p>
            ))}
          </AlertDescription>
        </Alert>
      ) : null}
      {submitError ? (
        <Alert variant="destructive">
          <AlertTitle>{t('appImport.connect.error')}</AlertTitle>
          <AlertDescription className="break-words whitespace-pre-wrap">
            {submitError.message}
          </AlertDescription>
        </Alert>
      ) : null}
      <p className="text-xs text-muted-foreground">
        {t('appImport.ssh.storedConfig')}
      </p>
      <Button
        type="button"
        disabled={!canTest || submitting}
        onClick={connectSession}
      >
        {submitting
          ? t('appImport.connect.submitting')
          : t('appImport.connect.submit')}
      </Button>
    </div>
  )
}
