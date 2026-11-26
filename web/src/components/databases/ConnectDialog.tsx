import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import {
  DownloadGlassIcon,
  EyeGlassIcon,
  EyeOffGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { EngineMark } from '@/components/EngineMark'
import { toast } from '@/components/ui/toast'
import {
  buildConnectSnippets,
  CONNECT_DOWNLOAD_FILES,
  connectEndpoint,
  tunnelCommand,
  type ConnectMode,
  connectDatabase,
  connectHasPassword,
  connectUser,
  envVarName,
  LOOPBACK_HOST,
  connectFrameworks,
  downloadContent,
  type ConnectDownload,
  type ConnectFramework,
  type ConnectInput,
} from '@/lib/connectSnippets'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import { useDatabaseNetwork } from '../../queries/databaseAccess'
import {
  credentialsStepUpScope,
  revealDatabasePassword,
} from '../../queries/databaseCredentials'
import type { DatabaseResource } from '../../types/databaseDetail'
import { StepUpDialog } from '../env/StepUpDialog'
import { CopyField } from '../traffic/CopyField'

const HIDE_AFTER_MS = 30_000
const DOWNLOADS: ConnectDownload[] = ['env', 'properties', 'yaml']

const MODES: ConnectMode[] = ['host', 'internal', 'remote']
const MAX_PORT = 65535

function saveFile(filename: string, text: string) {
  const url = URL.createObjectURL(new Blob([text], { type: 'text/plain' }))
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.click()
  URL.revokeObjectURL(url)
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground uppercase">{label}</dt>
      <dd className="truncate font-mono text-sm" title={value}>
        {value || '-'}
      </dd>
    </div>
  )
}

export function ConnectDialog({
  database,
  open,
  onOpenChange,
}: {
  database: DatabaseResource
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useTranslation('databases')
  const { data: network } = useDatabaseNetwork(database.name)
  const [mode, setMode] = useState<ConnectMode>('host')
  const [stack, setStack] = useState<ConnectFramework | null>(null)
  const [tunnelPort, setTunnelPort] = useState<number | null>(null)
  const [password, setPassword] = useState<string | null>(null)
  const [failure, setFailure] = useState<string | null>(null)
  const timer = useRef<number | null>(null)

  const hide = useCallback(() => {
    if (timer.current !== null) window.clearTimeout(timer.current)
    timer.current = null
    setPassword(null)
  }, [])
  useEffect(() => hide, [hide])

  const onGateError = useCallback(
    (e: unknown) => {
      const message = e instanceof Error ? e.message : t('connect.revealFailed')
      setFailure(message)
      toast.add({ title: message, type: 'error' })
    },
    [t],
  )
  const gate = useStepUpGate(onGateError)

  const reveal = async () => {
    setFailure(null)
    await gate.guard(credentialsStepUpScope(database.name), async () => {
      setPassword(await revealDatabasePassword(database.name))
      timer.current = window.setTimeout(hide, HIDE_AFTER_MS)
    })
  }

  const engine = database.engine
  const hasPassword = connectHasPassword(engine)
  const published = network?.published
  const publishedPort = published?.host_port ?? database.public_port
  const isPublished =
    Boolean(published) || Boolean(database.publicly_accessible)
  const tls = network?.tls.enabled ?? database.tls_enabled ?? false
  const loopbackOnly =
    database.public_bind_address === 'private' ||
    (published?.bind.length
      ? published.bind.every((b) => b === LOOPBACK_HOST)
      : false)
  const internal = network
    ? { host: network.internal.host, port: network.internal.port }
    : null
  const localPort = tunnelPort ?? internal?.port ?? null
  const remote =
    isPublished && publishedPort && !loopbackOnly
      ? { host: window.location.hostname, port: publishedPort }
      : null

  const endpoint =
    internal && localPort
      ? connectEndpoint({ mode, internal, tunnelPort: localPort, remote })
      : null

  const input: ConnectInput | null = endpoint
    ? {
        engine,
        host: endpoint.host,
        port: endpoint.port,
        database: connectDatabase(engine, database.name),
        user: connectUser(engine, database.name),
        password,
        tls,
      }
    : null
  const snippets = input ? buildConnectSnippets(input) : null
  const frameworks: ConnectFramework[] = connectFrameworks(engine)
  const activeStack =
    stack && frameworks.includes(stack) ? stack : (frameworks[0] ?? 'url')
  // Files carry the secret, so they unlock only after a reveal.
  const canDownload = !hasPassword || password !== null

  const download = (kind: ConnectDownload) => {
    if (!input) return
    const text = downloadContent(kind, input)
    if (text) saveFile(CONNECT_DOWNLOAD_FILES[kind], text)
  }

  const handleOpenChange = (next: boolean) => {
    if (!next) hide()
    onOpenChange(next)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent size="rich">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <EngineMark engine={engine} className="size-5 shrink-0" />
            {t('connect.title', { name: database.name })}
          </DialogTitle>
          <DialogDescription>{t('connect.description')}</DialogDescription>
        </DialogHeader>

        <div className="flex flex-wrap items-center gap-2">
          <span id="connect-where" className="text-sm font-medium">
            {t('connect.where')}
          </span>
          <div
            role="group"
            aria-labelledby="connect-where"
            className="flex flex-wrap items-center gap-2"
          >
            {MODES.map((id) => (
              <Button
                key={id}
                size="sm"
                variant={mode === id ? 'tinted' : 'outline'}
                aria-pressed={mode === id}
                onClick={() => {
                  setMode(id)
                }}
              >
                {t(`connect.mode.${id}`)}
              </Button>
            ))}
          </div>
        </div>
        {mode === 'host' ? (
          <div className="space-y-2">
            <div className="flex flex-wrap items-center gap-2">
              <label htmlFor="connect-tunnel-port" className="text-sm">
                {t('connect.localPort')}
              </label>
              <Input
                id="connect-tunnel-port"
                type="number"
                min={1}
                max={MAX_PORT}
                className="w-24"
                value={localPort ?? ''}
                onChange={(e) => {
                  const n = Number(e.target.value)
                  setTunnelPort(
                    Number.isInteger(n) && n > 0 && n <= MAX_PORT ? n : null,
                  )
                }}
              />
            </div>
            {localPort ? (
              <CopyField
                value={tunnelCommand(database.name, localPort)}
                label={t('connect.tunnelCommand')}
                className="w-full"
              />
            ) : null}
            <p className="text-xs text-muted-foreground">
              {t('connect.tunnelNote')}
            </p>
          </div>
        ) : null}
        {mode === 'internal' ? (
          <p className="text-xs text-muted-foreground">
            {t('connect.internalNote', { var: envVarName(engine) })}
          </p>
        ) : null}
        {mode === 'remote' && !remote ? (
          <div className="flex flex-wrap items-center gap-2">
            <p className="text-xs text-muted-foreground">
              {t('connect.remoteUnavailable')}
            </p>
            <Button
              size="sm"
              variant="outline"
              nativeButton={false}
              render={
                <Link
                  to="/databases/$name/networking"
                  params={{ name: database.name }}
                />
              }
            >
              {t('connect.openNetworking')}
            </Button>
          </div>
        ) : null}

        {input && snippets ? (
          <>
            <dl className="grid grid-cols-2 gap-x-4 gap-y-2 sm:grid-cols-5">
              <Detail label={t('connect.host')} value={input.host} />
              <Detail label={t('connect.port')} value={String(input.port)} />
              <Detail label={t('connect.database')} value={input.database} />
              <Detail label={t('connect.user')} value={input.user} />
              {hasPassword ? (
                <div className="min-w-0">
                  <dt className="text-xs text-muted-foreground uppercase">
                    {t('connect.password')}
                  </dt>
                  <dd className="flex items-center gap-1">
                    <span
                      className="min-w-0 flex-1 truncate font-mono text-sm"
                      title={password ?? undefined}
                    >
                      {password ?? '••••••••'}
                    </span>
                    <Button
                      size="icon-xs"
                      variant="ghost"
                      aria-label={
                        password
                          ? t('connect.hidePassword')
                          : t('connect.reveal')
                      }
                      aria-pressed={password !== null}
                      onClick={() => {
                        if (password) hide()
                        else void reveal()
                      }}
                    >
                      {password ? (
                        <EyeOffGlassIcon size="1.4em" aria-hidden="true" />
                      ) : (
                        <EyeGlassIcon size="1.4em" aria-hidden="true" />
                      )}
                    </Button>
                  </dd>
                </div>
              ) : null}
            </dl>
            <p className="-mt-2 text-xs text-muted-foreground">
              {!hasPassword
                ? t('connect.noPassword')
                : password
                  ? t('connect.autoHide')
                  : t('connect.placeholderNote')}
            </p>
            {failure ? (
              <Alert variant="destructive">
                <AlertDescription>{failure}</AlertDescription>
              </Alert>
            ) : null}

            <div
              role="radiogroup"
              aria-label={t('connect.stackLabel')}
              className="flex flex-wrap gap-1.5"
            >
              {frameworks.map((id) => (
                <Button
                  key={id}
                  size="sm"
                  role="radio"
                  aria-checked={activeStack === id}
                  variant={activeStack === id ? 'tinted' : 'outline'}
                  onClick={() => {
                    setStack(id)
                  }}
                >
                  {t(`connect.tab.${id}`)}
                </Button>
              ))}
            </div>
            <CopyField
              value={snippets[activeStack] ?? ''}
              label={t(`connect.tab.${activeStack}`)}
              multiline={activeStack !== 'url'}
              className="w-full"
            />

            <div className="flex flex-wrap items-center gap-2 border-t border-border pt-3">
              <span className="text-xs text-muted-foreground uppercase">
                {t('connect.download')}
              </span>
              {DOWNLOADS.filter((k) => downloadContent(k, input)).map((k) => (
                <Button
                  key={k}
                  size="sm"
                  variant="outline"
                  disabled={!canDownload}
                  onClick={() => {
                    download(k)
                  }}
                >
                  <DownloadGlassIcon size="1.4em" aria-hidden="true" />
                  {CONNECT_DOWNLOAD_FILES[k]}
                </Button>
              ))}
              {canDownload ? null : (
                <span className="text-xs text-muted-foreground">
                  {t('connect.downloadLocked')}
                </span>
              )}
            </div>
          </>
        ) : (
          <p className="text-sm text-muted-foreground">
            {t('connect.unavailable')}
          </p>
        )}

        <StepUpDialog
          scope={gate.pending?.scope ?? null}
          onVerified={() => {
            void gate.onVerified()
          }}
          onCancel={gate.cancel}
        />
      </DialogContent>
    </Dialog>
  )
}
