import { useMemo, useState, type ReactNode } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { PlusGlassIcon, TerminalGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
  const { data: rows, isPending, isError } = useCliTokenRows(true)
  const apps = useAppListOptional()
  const brand = useBrand()
  const revoke = useRevokeToken()
  const p = useCliPairing()
  const [target, setTarget] = useState<TokenResource | null>(null)
  const [rotateTarget, setRotateTarget] = useState<TokenResource | null>(null)
  const [setupOpen, setSetupOpen] = useState<boolean | null>(null)
  const windows = useMemo(() => detectCliPlatform() === 'windows', [])

  const connected = (rows ?? []).filter((r) => r.status !== 'expired').length
  const hasRows = (rows?.length ?? 0) > 0
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

  const manage = (
    <section aria-labelledby="cli-manage-title" className="space-y-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <FieldLabel as="span" id="cli-manage-title" className="text-foreground">
          {t('cliModal.manage.title')}
        </FieldLabel>
        {connected > 0 ? (
          <Button
            type="button"
            size="xs"
            variant="outline"
            aria-expanded={showSetup}
            onClick={() => {
              setSetupOpen(!showSetup)
            }}
          >
            <PlusGlassIcon size="1.4em" aria-hidden="true" />
            {t('cliModal.manage.connectAnother')}
          </Button>
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
          className="divide-y rounded-lg border"
        >
          {rows?.map(({ token: tok, status }) => {
            const origin = cliOrigin(tok.name)
            const chips = abilityChips(tok)
            return (
              <li
                key={tok.id}
                className="flex flex-wrap items-center justify-between gap-3 px-3 py-2"
              >
                <div className="min-w-0 flex-1 space-y-1">
                  <div className="flex min-w-0 items-center gap-2">
                    <span
                      aria-hidden="true"
                      className={cn(
                        'size-2 shrink-0 rounded-full',
                        STATUS_DOT[status],
                      )}
                    />
                    <p className="truncate text-sm font-medium">{tok.name}</p>
                    <span className="shrink-0 text-xs text-muted-foreground">
                      {t(`cliModal.manage.status.${status}`)}
                    </span>
                  </div>
                  <p className="text-xs text-muted-foreground">
                    {origin ? t(`cliModal.manage.origin.${origin}`) : ''}
                    {' · '}
                    {t('cliModal.manage.created', {
                      when: formatAbsoluteTime(tok.created_at),
                    })}
                    {' · '}
                    {tok.last_used_at
                      ? t('cliModal.manage.lastUsed', {
                          when: formatRelativeTime(tok.last_used_at),
                        })
                      : t('cliModal.manage.neverUsed')}
                    {tok.last_ip
                      ? ` · ${t('cliModal.manage.lastIp', { ip: tok.last_ip })}`
                      : ''}
                    {' · '}
                    {tok.expires_at
                      ? t(
                          status === 'expired'
                            ? 'cliModal.manage.expired'
                            : 'cliModal.manage.expires',
                          { when: formatAbsoluteTime(tok.expires_at) },
                        )
                      : t('cliModal.manage.noExpiry')}
                  </p>
                  <ul
                    aria-label={t('cliModal.manage.abilities')}
                    className="flex flex-wrap gap-1"
                  >
                    {chips.map((c) => (
                      <li key={c}>
                        <Badge
                          variant={
                            c === 'abilityRoot' ? 'destructive' : 'outline'
                          }
                        >
                          {t(`cliModal.manage.${c}`)}
                        </Badge>
                      </li>
                    ))}
                  </ul>
                </div>
                <div className="flex shrink-0 gap-2">
                  <Button
                    type="button"
                    size="xs"
                    variant="outline"
                    aria-label={t('cliModal.manage.rotateLabel', {
                      name: tok.name,
                    })}
                    onClick={() => {
                      setRotateTarget(tok)
                    }}
                  >
                    {t('cliModal.manage.rotate')}
                  </Button>
                  <Button
                    type="button"
                    size="xs"
                    variant="outline"
                    aria-label={t('cliModal.manage.revokeLabel', {
                      name: tok.name,
                    })}
                    onClick={() => {
                      setTarget(tok)
                    }}
                  >
                    {t('cliModal.manage.revoke')}
                  </Button>
                </div>
              </li>
            )
          })}
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
        <>
          {manage}
          {showSetup ? setup : null}
          {tryIt}
        </>
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
