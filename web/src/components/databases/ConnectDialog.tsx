import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import {
  DownloadGlassIcon,
  EyeGlassIcon,
  EyeOffGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { EngineMark } from '@/components/EngineMark'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toast } from '@/components/ui/toast'
import {
  buildConnectSnippets,
  CONNECT_DOWNLOAD_FILES,
  connectDatabase,
  connectHasPassword,
  connectUser,
  connectFrameworks,
  downloadContent,
  type ConnectDownload,
  type ConnectFramework,
  type ConnectInput,
} from '@/lib/connectSnippets'
import { useStepUpGate } from '../../hooks/useStepUpGate'
import {
  databaseAccessKeys,
  useDatabaseNetwork,
} from '../../queries/databaseAccess'
import {
  credentialsStepUpScope,
  revealDatabasePassword,
} from '../../queries/databaseCredentials'
import { useSetDatabasePublicAccess } from '../../queries/databases'
import type { DatabaseResource } from '../../types/databaseDetail'
import { StepUpDialog } from '../env/StepUpDialog'
import { CopyField } from '../traffic/CopyField'

const HIDE_AFTER_MS = 30_000
const LOOPBACK = '127.0.0.1'
const DOWNLOADS: ConnectDownload[] = ['env', 'properties', 'yaml']

type Target = 'machine' | 'apps'

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
  const queryClient = useQueryClient()
  const { data: network } = useDatabaseNetwork(database.name)
  const publish = useSetDatabasePublicAccess()
  const [target, setTarget] = useState<Target | null>(null)
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
  const active: Target = target ?? (isPublished ? 'machine' : 'apps')
  const tls = network?.tls.enabled ?? database.tls_enabled ?? false
  const loopbackOnly =
    database.public_bind_address === 'private' ||
    (published?.bind.length
      ? published.bind.every((b) => b === LOOPBACK)
      : false)

  const endpoint =
    active === 'machine' && publishedPort
      ? {
          host: loopbackOnly ? LOOPBACK : window.location.hostname,
          port: publishedPort,
        }
      : network
        ? { host: network.internal.host, port: network.internal.port }
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

  const runPublish = () => {
    publish.mutate(
      { name: database.name },
      {
        onSuccess: () => {
          setTarget('machine')
          void queryClient.invalidateQueries({
            queryKey: databaseAccessKeys.network(database.name),
          })
        },
        onError: (e) => {
          toast.add({ title: e.message, type: 'error' })
        },
      },
    )
  }

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

        <div
          role="group"
          aria-label={t('connect.targetLabel')}
          className="flex flex-wrap items-center gap-2"
        >
          {(['machine', 'apps'] as const).map((id) => (
            <Button
              key={id}
              size="sm"
              variant={active === id ? 'tinted' : 'outline'}
              aria-pressed={active === id}
              onClick={() => setTarget(id)}
            >
              {t(`connect.target.${id}`)}
            </Button>
          ))}
          {active === 'machine' ? (
            isPublished && publishedPort ? (
              <Badge variant="success">
                {t('connect.published', {
                  address: `${loopbackOnly ? LOOPBACK : window.location.hostname}:${publishedPort}`,
                })}
              </Badge>
            ) : (
              <Button
                size="sm"
                disabled={publish.isPending}
                aria-busy={publish.isPending}
                onClick={runPublish}
              >
                {publish.isPending
                  ? t('connect.publishing')
                  : t('connect.publish')}
              </Button>
            )
          ) : null}
        </div>
        {active === 'machine' && !isPublished ? (
          <p className="text-xs text-muted-foreground">
            {t('connect.publishHint')}
          </p>
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

            <CopyField
              value={snippets.url ?? ''}
              label={t('connect.url')}
              className="w-full"
            />

            <Tabs defaultValue={frameworks[0]}>
              <TabsList className="max-w-full">
                {frameworks.map((id) => (
                  <TabsTrigger key={id} value={id}>
                    {t(`connect.tab.${id}`)}
                  </TabsTrigger>
                ))}
              </TabsList>
              {frameworks.map((id) => (
                <TabsContent key={id} value={id} className="pt-2">
                  <CopyField
                    value={snippets[id] ?? ''}
                    label={t(`connect.tab.${id}`)}
                    multiline={id !== 'url'}
                  />
                </TabsContent>
              ))}
            </Tabs>

            <div className="flex flex-wrap items-center gap-2 border-t border-border pt-3">
              <span className="text-xs text-muted-foreground uppercase">
                {t('connect.download')}
              </span>
              {DOWNLOADS.filter((k) => downloadContent(k, input)).map((k) => (
                <Button
                  key={k}
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    download(k)
                  }}
                >
                  <DownloadGlassIcon size="1.4em" aria-hidden="true" />
                  {CONNECT_DOWNLOAD_FILES[k]}
                </Button>
              ))}
            </div>
          </>
        ) : (
          <p className="text-sm text-muted-foreground">
            {t('connect.unavailable')}
          </p>
        )}

        <details className="rounded-md border border-border p-3">
          <summary className="cursor-pointer text-sm font-medium">
            {t('connectDrawer.tunnelTitle')}
          </summary>
          <div className="mt-3 space-y-2">
            <p className="text-sm text-muted-foreground">
              {t('connectDrawer.tunnelBody')}
            </p>
            <CopyField
              value={`levelrail-cli databases connect ${database.name} --launch`}
              label={t('connectDrawer.copyCommand')}
            />
          </div>
        </details>
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
