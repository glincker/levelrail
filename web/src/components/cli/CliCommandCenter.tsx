import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon, TerminalGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { FieldLabel } from '@/components/ui/typography'
import {
  TerminalBlock,
  TerminalCopyButton,
  type TerminalLine,
} from '@/components/ui/terminal-block'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { useBrand } from '../../hooks/useBrand'
import { useCliPairing, formatLeft } from '../../hooks/useCliPairing'
import { cliOrigin, type CliStatus } from '../../lib/cliConnection'
import { CLI_INSTALL_COMMAND } from '../../lib/cliInstall'
import { useAppListOptional } from '../../queries/apps'
import { CLI_INSTALL_COMMAND_WINDOWS, detectCliPlatform } from './cliPlatform'
import { formatAbsoluteTime, formatRelativeTime } from '../../lib/relativeTime'
import { myCredentialsKey, useCliTokenRows } from '../../queries/myCredentials'
import { useRevokeToken } from '../../queries/tokens'
import { RotateCliTokenDialog } from './RotateCliTokenDialog'
import type { TokenResource } from '../../types/token'

const STATUS_DOT: Record<CliStatus, string> = {
  active: 'bg-tone-success-solid',
  expiring: 'bg-tone-warning-solid',
  expired: 'bg-tone-danger-solid',
}

type AbilityKey =
  | 'abilityRead'
  | 'abilityWrite'
  | 'abilityDeploy'
  | 'abilitySensitive'
  | 'abilityRoot'

// Collapses the six API abilities into the four chips people reason about.
function abilityChips(tok: TokenResource): AbilityKey[] {
  const a = tok.abilities ?? []
  if (a.includes('root')) {
    return ['abilityRoot']
  }
  const out: AbilityKey[] = []
  if (a.includes('read')) out.push('abilityRead')
  if (a.includes('write')) out.push('abilityWrite')
  if (a.includes('deploy')) out.push('abilityDeploy')
  if (a.some((x) => x.endsWith(':sensitive') || x === 'secrets:reveal')) {
    out.push('abilitySensitive')
  }
  return out
}

type Pairing = ReturnType<typeof useCliPairing>

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="truncate text-sm">{value}</dd>
    </div>
  )
}

function CliCard({
  tok,
  status,
  latest,
  onRotate,
  onRevoke,
}: {
  tok: TokenResource
  status: CliStatus
  latest: boolean
  onRotate: (tok: TokenResource) => void
  onRevoke: (tok: TokenResource) => void
}) {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const origin = cliOrigin(tok.name)
  const expired = status === 'expired'
  return (
    <li
      className={cn(
        'flex min-w-0 flex-col gap-3 rounded-lg border bg-card p-3',
        expired && 'opacity-70',
      )}
    >
      <div className="flex min-w-0 items-start gap-2">
        <TerminalGlassIcon aria-hidden="true" className="size-8 shrink-0" />
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium">{tok.name}</p>
          <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <span
              aria-hidden="true"
              className={cn('size-2 shrink-0 rounded-full', STATUS_DOT[status])}
            />
            {t(`cliModal.manage.status.${status}`)}
            {origin ? ` · ${t(`cliModal.manage.origin.${origin}`)}` : ''}
          </p>
        </div>
        {latest ? (
          <Badge variant="success">{t('cliModal.summary.mostRecent')}</Badge>
        ) : null}
      </div>
      <dl className="grid grid-cols-2 gap-x-3 gap-y-1.5">
        <Fact
          label={t('cliModal.card.lastUsed')}
          value={
            tok.last_used_at
              ? formatRelativeTime(tok.last_used_at)
              : t('cliModal.manage.neverUsed')
          }
        />
        <Fact
          label={t('cliModal.card.from')}
          value={tok.last_ip ?? t('cliModal.card.unknown')}
        />
        <Fact
          label={t('cliModal.card.created')}
          value={formatAbsoluteTime(tok.created_at)}
        />
        <Fact
          label={t(
            expired ? 'cliModal.card.expiredAt' : 'cliModal.card.expires',
          )}
          value={
            tok.expires_at
              ? formatAbsoluteTime(tok.expires_at)
              : t('cliModal.manage.noExpiry')
          }
        />
      </dl>
      <ul
        aria-label={t('cliModal.manage.abilities')}
        className="flex flex-wrap gap-1"
      >
        {abilityChips(tok).map((c) => (
          <li key={c}>
            <Badge variant={c === 'abilityRoot' ? 'destructive' : 'outline'}>
              {t(`cliModal.manage.${c}`)}
            </Badge>
          </li>
        ))}
      </ul>
      <div className="mt-auto flex gap-2">
        <Button
          type="button"
          size="xs"
          variant="outline"
          aria-label={t('cliModal.manage.rotateLabel', { name: tok.name })}
          onClick={() => {
            onRotate(tok)
          }}
        >
          {t('cliModal.manage.rotate')}
        </Button>
        <Button
          type="button"
          size="xs"
          variant="outline"
          aria-label={t('cliModal.manage.revokeLabel', { name: tok.name })}
          onClick={() => {
            onRevoke(tok)
          }}
        >
          {t('cliModal.manage.revoke')}
        </Button>
      </div>
    </li>
  )
}

function Step({
  n,
  title,
  status,
  children,
}: {
  n: number
  title: string
  status: string
  children: ReactNode
}) {
  return (
    <section aria-labelledby={`cli-step-${n}`} className="min-w-0 space-y-1.5">
      <div className="flex items-baseline justify-between gap-2">
        <FieldLabel as="span" id={`cli-step-${n}`} className="text-foreground">
          {n}. {title}
        </FieldLabel>
        <span className="text-xs text-muted-foreground">{status}</span>
      </div>
      {children}
    </section>
  )
}

function InstallBlock({ windows }: { windows: boolean }) {
  const { t } = useTranslation('signIn', { useSuspense: false })
  return (
    <TerminalBlock
      label={t('cliModal.install.label')}
      prompt={windows ? '>' : '$'}
      lines={[
        {
          kind: 'comment',
          text: t(
            windows
              ? 'cliModal.install.commentWindows'
              : 'cliModal.install.comment',
          ),
        },
        { kind: 'command', text: installCommand(windows) },
      ]}
    />
  )
}

function installCommand(windows: boolean): string {
  return windows ? CLI_INSTALL_COMMAND_WINDOWS : CLI_INSTALL_COMMAND
}

function ConnectBlock({ p }: { p: Pairing }) {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const lines: TerminalLine[] = p.live
    ? [
        {
          kind: 'comment',
          text: t('cliModal.connect.comment', {
            server: p.pairing?.server_url ?? '',
          }),
        },
        { kind: 'command', text: p.command },
      ]
    : [
        {
          kind: 'comment',
          text: t(
            p.expired ? 'cliModal.connect.expired' : 'cliModal.connect.pending',
          ),
        },
      ]
  return (
    <TerminalBlock
      label={t('connectCli.commandLabel')}
      lines={lines}
      footer={
        <>
          {p.live ? (
            <span role="timer" className="text-xs text-muted-foreground">
              {t('connectCli.expiresIn', { time: formatLeft(p.left) })}
            </span>
          ) : null}
          <Button
            type="button"
            size="xs"
            variant={p.live ? 'ghost' : 'default'}
            disabled={p.pending}
            onClick={p.generate}
          >
            {p.pairing ? t('connectCli.regenerate') : t('connectCli.generate')}
          </Button>
        </>
      }
    />
  )
}

type TryKey = 'status' | 'list' | 'logs' | 'wait' | 'open' | 'up'

function tryLines(
  bin: string,
  app: string,
  label: (key: TryKey) => string,
): TerminalLine[] {
  // Each one is a real subcommand of the CLI (see its --help).
  const rows: [TryKey, string][] = [
    ['status', `${bin} status`],
    ['list', `${bin} apps list`],
    ['logs', `${bin} apps logs ${app} --follow`],
    ['wait', `${bin} apps wait ${app}`],
    ['open', `${bin} open ${app}`],
    ['up', `${bin} up`],
  ]
  return rows.flatMap(([key, cmd]): TerminalLine[] => [
    { kind: 'comment', text: label(key) },
    { kind: 'command', text: cmd },
  ])
}

export default function CliCommandCenter({
  onFullPage,
}: {
  onFullPage: () => void
}) {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const qc = useQueryClient()
  const p = useCliPairing()
  const { data: rows, isPending, isError } = useCliTokenRows(true, p.live)
  const apps = useAppListOptional()
  const brand = useBrand()
  const revoke = useRevokeToken()
  const [target, setTarget] = useState<TokenResource | null>(null)
  const [rotateTarget, setRotateTarget] = useState<TokenResource | null>(null)
  const [setupOpen, setSetupOpen] = useState<boolean | null>(null)
  const windows = useMemo(() => detectCliPlatform() === 'windows', [])

  const connected = (rows ?? []).filter((r) => r.status !== 'expired').length
  const hasRows = (rows?.length ?? 0) > 0
  const seen = useRef<number | null>(null)
  useEffect(() => {
    if (isPending) {
      return
    }
    // A count that grows while the modal is open means the pairing worked.
    if (seen.current !== null && connected > seen.current) {
      toast.add({ title: t('cliModal.connectedToast'), type: 'success' })
      setSetupOpen(false)
    }
    seen.current = connected
  }, [connected, isPending, t])
  // Connected: lead with the CLIs and fold install and connect away.
  const showSetup = setupOpen ?? connected === 0
  const app = apps.data?.[0]?.name ?? '<app>'
  const tryText = tryLines(brand.BinaryName, app, (key) =>
    t(`cliModal.commands.${key}`),
  )
  const allText = [
    installCommand(windows),
    ...(p.live ? [p.command] : []),
    ...tryText.filter((l) => l.kind === 'command').map((l) => l.text),
  ].join('\n')

  const setup = (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <FieldLabel>{t('cliModal.started.title')}</FieldLabel>
        <TerminalCopyButton
          text={allText}
          label={t('cliModal.copyAll')}
          variant="ghost"
        />
      </div>
      <Step
        n={1}
        title={t('cliModal.install.title')}
        status={t(
          connected > 0 ? 'cliModal.step.done' : 'cliModal.step.installUnknown',
        )}
      >
        <InstallBlock windows={windows} />
      </Step>
      <Step
        n={2}
        title={t('cliModal.connect.title')}
        status={t(
          connected > 0
            ? 'cliModal.step.connected'
            : p.live
              ? 'cliModal.step.ready'
              : 'cliModal.step.notConnected',
        )}
      >
        <div aria-live="polite">
          <ConnectBlock p={p} />
        </div>
      </Step>
    </div>
  )

  const tryIt = (
    <Step
      n={3}
      title={t('cliModal.commands.title')}
      status={t(
        connected > 0 ? 'cliModal.step.ready' : 'cliModal.step.afterConnect',
      )}
    >
      <TerminalBlock label={t('cliModal.commands.title')} lines={tryText} />
    </Step>
  )

  const newest = useMemo(() => {
    const used = (rows ?? []).filter((r) => r.token.last_used_at)
    used.sort((x, y) =>
      (y.token.last_used_at ?? '').localeCompare(x.token.last_used_at ?? ''),
    )
    return used[0]?.token
  }, [rows])

  const manage = (
    <section aria-labelledby="cli-manage-title" className="space-y-2">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <FieldLabel as="span" id="cli-manage-title" className="text-foreground">
          {t('cliModal.manage.title')}
        </FieldLabel>
        {newest?.last_used_at ? (
          <span className="text-xs text-muted-foreground">
            {t('cliModal.summary.lastActivity', {
              when: formatRelativeTime(newest.last_used_at),
            })}
          </span>
        ) : null}
      </div>
      {isError ? (
        <p role="alert" className="text-sm text-muted-foreground">
          {t('cliModal.manage.loadFailed')}
        </p>
      ) : null}
      {hasRows ? (
        <ul
          aria-label={t('cliModal.manage.listLabel')}
          className="grid gap-2 sm:grid-cols-2"
        >
          {rows?.map(({ token: tok, status }) => (
            <CliCard
              key={tok.id}
              tok={tok}
              status={status}
              latest={rows.length > 1 && tok.id === newest?.id}
              onRotate={setRotateTarget}
              onRevoke={setTarget}
            />
          ))}
        </ul>
      ) : null}
    </section>
  )

  return (
    <div className="min-w-0 space-y-4">
      <DialogHeader>
        <div className="flex flex-wrap items-center gap-2 pr-8">
          <TerminalGlassIcon aria-hidden="true" className="size-6" />
          <DialogTitle>{t('cliModal.title')}</DialogTitle>
          <Badge
            data-testid="cli-status-chip"
            variant={connected > 0 ? 'success' : 'muted'}
          >
            {isPending
              ? t('cliModal.status.loading')
              : connected > 0
                ? t('cliModal.status.connected', { count: connected })
                : t('cliModal.status.none')}
          </Badge>
        </div>
        <DialogDescription>{t('cliModal.subtitle')}</DialogDescription>
      </DialogHeader>

      {connected > 0 ? (
        <Tabs
          value={showSetup ? 'setup' : 'clis'}
          onValueChange={(v) => {
            setSetupOpen(v === 'setup')
          }}
        >
          <TabsList aria-label={t('cliModal.view.label')}>
            <TabsTrigger value="clis">
              {t('cliModal.view.clis', { count: connected })}
            </TabsTrigger>
            <TabsTrigger value="setup">
              <PlusGlassIcon size="1.4em" aria-hidden="true" />
              {t('cliModal.manage.connectAnother')}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="clis" className="space-y-4">
            {manage}
            {tryIt}
          </TabsContent>
          <TabsContent value="setup" className="space-y-4">
            {setup}
          </TabsContent>
        </Tabs>
      ) : (
        <>
          {setup}
          {tryIt}
          {hasRows ? manage : null}
        </>
      )}

      <Link
        to="/settings/cli-access"
        onClick={onFullPage}
        className="inline-block rounded-sm text-sm text-primary underline-offset-4 hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
      >
        {t('cliModal.manage.fullPage')}
      </Link>

      {rotateTarget ? (
        <RotateCliTokenDialog
          token={rotateTarget}
          onClose={() => {
            setRotateTarget(null)
          }}
        />
      ) : null}

      <ConfirmDialog
        open={target !== null}
        onOpenChange={(next) => {
          if (!next) {
            setTarget(null)
          }
        }}
        title={t('cliModal.manage.revokeTitle')}
        description={t('cliModal.manage.revokeBody', {
          name: target?.name ?? '',
        })}
        tone="destructive"
        confirmLabel={t('cliModal.manage.revokeConfirm')}
        pendingLabel={t('cliModal.manage.revoking')}
        cancelLabel={t('cliModal.manage.revokeCancel')}
        pending={revoke.isPending}
        onConfirm={() => {
          if (!target) {
            return
          }
          revoke.mutate(target.id, {
            onSuccess: () => {
              void qc.invalidateQueries({ queryKey: myCredentialsKey })
              toast.add({
                title: t('cliModal.manage.revoked'),
                type: 'success',
              })
              setTarget(null)
            },
            onError: () => {
              toast.add({
                title: t('cliModal.manage.revokeFailed'),
                type: 'error',
              })
            },
          })
        }}
      />
    </div>
  )
}
