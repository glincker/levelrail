import { useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CaretDownIcon,
  CheckIcon,
  CopyIcon,
  PlusIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { CommandBlock } from '@/components/ui/code-block'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import {
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { toast } from '@/components/ui/toast'
import { cn } from '@/lib/utils'
import { CopyCommand } from '../settings/CopyCommand'
import { useBrand } from '../../hooks/useBrand'
import { useCliPairing, formatLeft } from '../../hooks/useCliPairing'
import { cliOrigin, type CliStatus } from '../../lib/cliConnection'
import { CLI_INSTALL_COMMAND } from '../../lib/cliInstall'
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

function ConnectStep({ p }: { p: Pairing }) {
  const { t } = useTranslation('signIn', { useSuspense: false })
  return (
    <section aria-labelledby="cli-connect-title" className="space-y-2">
      <h3 id="cli-connect-title" className="text-sm font-semibold">
        {t('cliModal.connect.title')}
      </h3>
      <p className="text-sm text-muted-foreground">
        {t('cliModal.connect.lead')}
      </p>
      <div aria-live="polite" className="space-y-2">
        {p.live ? (
          <>
            <code
              aria-label={t('connectCli.commandLabel')}
              data-testid="connect-cli-command"
              className="block select-all overflow-x-auto rounded-md bg-muted px-3 py-2 font-mono text-xs"
            >
              {p.command}
            </code>
            <p className="text-xs text-muted-foreground">
              {t('connectCli.expiresIn', { time: formatLeft(p.left) })}
            </p>
          </>
        ) : p.expired ? (
          <p className="text-sm text-muted-foreground">
            {t('connectCli.expired')}
          </p>
        ) : null}
      </div>
      <div className="flex flex-wrap gap-2">
        {p.live ? (
          <Button type="button" size="sm" onClick={p.copy}>
            {p.copied ? (
              <CheckIcon aria-hidden="true" />
            ) : (
              <CopyIcon aria-hidden="true" />
            )}
            {t('connectCli.copy')}
          </Button>
        ) : null}
        <Button
          type="button"
          size="sm"
          variant={p.live ? 'outline' : 'default'}
          disabled={p.pending}
          onClick={p.generate}
        >
          {p.pairing ? t('connectCli.regenerate') : t('connectCli.generate')}
        </Button>
      </div>
    </section>
  )
}

function CommonCommands() {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const brand = useBrand()
  const bin = brand.BinaryName
  // Each one is a real subcommand of the CLI (see its --help).
  const commands = [
    { key: 'status', cmd: `${bin} status` },
    { key: 'list', cmd: `${bin} apps list` },
    { key: 'logs', cmd: `${bin} apps logs <app> --follow` },
    { key: 'wait', cmd: `${bin} apps wait <app>` },
    { key: 'open', cmd: `${bin} open <app>` },
    { key: 'up', cmd: `${bin} up` },
  ] as const
  return (
    <section aria-labelledby="cli-commands-title" className="space-y-2">
      <h3 id="cli-commands-title" className="text-sm font-semibold">
        {t('cliModal.commands.title')}
      </h3>
      <p className="text-sm text-muted-foreground">
        {t('cliModal.commands.lead')}
      </p>
      <ul className="grid gap-2 md:grid-cols-2">
        {commands.map((c) => (
          <li key={c.key} className="min-w-0 space-y-1">
            <p className="text-xs text-muted-foreground">
              {t(`cliModal.commands.${c.key}`)}
            </p>
            <CommandBlock
              code={c.cmd}
              label={t('cliModal.commands.copy', { command: c.cmd })}
            />
          </li>
        ))}
      </ul>
    </section>
  )
}

export default function CliCommandCenter({
  onFullPage,
}: {
  onFullPage: () => void
}) {
  const { t } = useTranslation('signIn', { useSuspense: false })
  const qc = useQueryClient()
  const { data: rows, isPending, isError } = useCliTokenRows(true)
  const revoke = useRevokeToken()
  const p = useCliPairing()
  const [target, setTarget] = useState<TokenResource | null>(null)
  const [rotateTarget, setRotateTarget] = useState<TokenResource | null>(null)
  const [setupOpen, setSetupOpen] = useState<boolean | null>(null)

  const connected = (rows ?? []).filter((r) => r.status !== 'expired').length
  const hasRows = (rows?.length ?? 0) > 0
  // Setup stays open until a CLI is connected, then collapses unless the
  // user chose otherwise.
  const showSetup = setupOpen ?? !hasRows

  return (
    <div className="min-w-0 space-y-5">
      <DialogHeader>
        <div className="flex flex-wrap items-center gap-2 pr-8">
          <TerminalWindowIcon aria-hidden="true" className="size-5" />
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

      <section
        aria-labelledby="cli-started-title"
        className="rounded-lg border"
      >
        <h3 id="cli-started-title" className="text-sm font-semibold">
          <button
            type="button"
            aria-expanded={showSetup}
            aria-controls="cli-started-body"
            onClick={() => {
              setSetupOpen(!showSetup)
            }}
            className="flex w-full items-center justify-between gap-2 rounded-lg px-3 py-2 text-left focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
          >
            <span className="flex items-center gap-2">
              {t('cliModal.started.title')}
              {hasRows ? (
                <Badge variant="success">
                  <CheckIcon aria-hidden="true" />
                  {t('cliModal.started.done')}
                </Badge>
              ) : null}
            </span>
            <CaretDownIcon
              aria-hidden="true"
              className={cn(
                'size-4 transition-transform motion-reduce:transition-none',
                showSetup && 'rotate-180',
              )}
            />
          </button>
        </h3>
        <div
          id="cli-started-body"
          hidden={!showSetup}
          className="grid gap-5 border-t px-3 py-3 md:grid-cols-2"
        >
          <section
            aria-labelledby="cli-install-title"
            className="min-w-0 space-y-2"
          >
            <h3 id="cli-install-title" className="text-sm font-semibold">
              {t('cliModal.install.title')}
            </h3>
            <p className="text-sm text-muted-foreground">
              {t('cliModal.install.lead')}
            </p>
            <CopyCommand
              label={t('cliModal.install.label')}
              command={CLI_INSTALL_COMMAND}
            />
          </section>
          <div className="min-w-0">
            <ConnectStep p={p} />
          </div>
        </div>
      </section>

      <section aria-labelledby="cli-manage-title" className="space-y-2">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h3 id="cli-manage-title" className="text-sm font-semibold">
            {t('cliModal.manage.title')}
          </h3>
          {hasRows ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => {
                setSetupOpen(true)
                if (!p.live) {
                  p.generate()
                }
              }}
            >
              <PlusIcon aria-hidden="true" />
              {t('cliModal.manage.connectAnother')}
            </Button>
          ) : null}
        </div>
        {isPending ? (
          <p className="text-sm text-muted-foreground">
            {t('cliModal.manage.loading')}
          </p>
        ) : isError ? (
          <p role="alert" className="text-sm text-muted-foreground">
            {t('cliModal.manage.loadFailed')}
          </p>
        ) : !hasRows ? (
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-dashed px-3 py-4">
            <p className="text-sm text-muted-foreground">
              {t('cliModal.manage.empty')}
            </p>
            <Button
              type="button"
              size="sm"
              disabled={p.pending}
              onClick={() => {
                setSetupOpen(true)
                p.generate()
              }}
            >
              {t('cliModal.manage.emptyAction')}
            </Button>
          </div>
        ) : (
          <ul
            aria-label={t('cliModal.manage.listLabel')}
            className="divide-y rounded-lg border"
          >
            {rows.map(({ token: tok, status }) => {
              const origin = cliOrigin(tok.name)
              const chips = abilityChips(tok)
              return (
                <li
                  key={tok.id}
                  className="flex flex-wrap items-center justify-between gap-3 px-3 py-3"
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
                      size="sm"
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
                      size="sm"
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
        )}
        <Link
          to="/settings/cli-access"
          onClick={onFullPage}
          className="inline-block rounded-sm text-sm text-primary underline-offset-4 hover:underline focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        >
          {t('cliModal.manage.fullPage')}
        </Link>
      </section>

      <CommonCommands />

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
