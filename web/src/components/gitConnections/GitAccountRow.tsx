import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleXGlassIcon,
  ExternalLinkGlassIcon,
  KeyGlassIcon,
  PencilGlassIcon,
  PlugGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { StatusBadge } from '@/components/ui/status-badge'
import { toast } from '@/components/ui/toast'
import { BrandIcon } from '../BrandIcon'
import type { GitAccount, GitAccountHealth } from '../../lib/gitAccounts'
import {
  useDisconnectGitConnection,
  useRenameGitConnection,
} from '../../queries/gitConnections'
import { useDeleteGitHubAppInstallation } from '../../queries/githubApp'

const BRAND = {
  github: 'github',
  gitlab: 'gitlab',
  bitbucket: 'bitbucket',
  bitbucket_server: 'bitbucket',
  gitea: 'gitea',
  forgejo: 'forgejo',
} as const

export function HealthBadge({
  health,
}: Readonly<{ health: GitAccountHealth }>) {
  const { t } = useTranslation('settings')
  switch (health) {
    case 'needs_reconnect':
      return (
        <StatusBadge
          tone="danger"
          icon={CircleXGlassIcon}
          label={t('gitConnectionsHub.health.needsReconnect')}
        />
      )
    case 'not_authorized':
      return (
        <StatusBadge
          tone="warning"
          icon={KeyGlassIcon}
          label={t('gitConnectionsHub.health.notAuthorized')}
        />
      )
    case 'rate_limited':
      return (
        <StatusBadge
          tone="warning"
          icon={TriangleAlertGlassIcon}
          label={t('gitConnectionsHub.health.rateLimited')}
        />
      )
    default:
      return (
        <StatusBadge
          tone="success"
          icon={CircleCheckGlassIcon}
          label={t('gitConnectionsHub.health.connected')}
        />
      )
  }
}

function Stat({ label, value }: Readonly<{ label: string; value?: number }>) {
  return (
    <div className="min-w-16">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="font-mono text-sm text-foreground">{value ?? '-'}</dd>
    </div>
  )
}

export function GitAccountRow({ account }: Readonly<{ account: GitAccount }>) {
  const { t } = useTranslation('settings')
  const rename = useRenameGitConnection()
  const disconnectConnection = useDisconnectGitConnection()
  const disconnectInstallation = useDeleteGitHubAppInstallation()
  const [renaming, setRenaming] = useState(false)
  const [label, setLabel] = useState(account.label)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const disconnect =
    account.githubInstallationId === undefined
      ? disconnectConnection
      : disconnectInstallation
  const needsAction = account.health !== 'ok'

  function submitRename() {
    const next = label.trim()
    if (!account.connectionId || next === '' || next === account.label) {
      setRenaming(false)
      return
    }
    rename.mutate(
      { id: account.connectionId, label: next },
      {
        onSuccess: () => setRenaming(false),
        onError: (error) =>
          toast.add({
            title: t('gitConnections.toast.renameFailed'),
            description: error.message,
            type: 'error',
          }),
      },
    )
  }

  function confirmDisconnect() {
    const done = () => {
      setConfirmOpen(false)
      toast.add({
        title: t('gitConnections.toast.disconnected'),
        type: 'success',
      })
    }
    if (account.githubInstallationId !== undefined) {
      disconnectInstallation.mutate(account.githubInstallationId, {
        onSuccess: done,
      })
      return
    }
    if (account.connectionId) {
      disconnectConnection.mutate(account.connectionId, {
        onSuccess: (result) => {
          setConfirmOpen(false)
          toast.add({
            title: t('gitConnections.toast.disconnected'),
            description: result.affected_sources?.length
              ? t('gitConnectionsHub.disconnect.affected', {
                  apps: result.affected_sources.join(', '),
                })
              : result.note,
            type: 'success',
          })
        },
      })
    }
  }

  return (
    <li className="space-y-3 py-4 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <BrandIcon
            name={BRAND[account.provider]}
            className="mt-0.5 size-8 shrink-0"
          />
          <div className="min-w-0 space-y-1">
            {renaming ? (
              <form
                className="flex items-center gap-2"
                onSubmit={(event) => {
                  event.preventDefault()
                  submitRename()
                }}
              >
                <Input
                  value={label}
                  autoFocus
                  maxLength={80}
                  aria-label={t('gitConnections.rename.label')}
                  onChange={(event) => setLabel(event.target.value)}
                  className="h-8 w-56"
                />
                <Button type="submit" size="sm" disabled={rename.isPending}>
                  {t('gitConnections.rename.save')}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    setLabel(account.label)
                    setRenaming(false)
                  }}
                >
                  {t('gitConnections.rename.cancel')}
                </Button>
              </form>
            ) : (
              <div className="flex flex-wrap items-center gap-2">
                <span className="text-sm font-medium text-foreground">
                  {account.name}
                </span>
                {account.label !== account.name ? (
                  <Badge variant="muted">{account.label}</Badge>
                ) : null}
                {account.isDefault ? (
                  <Badge variant="muted">{t('gitConnections.default')}</Badge>
                ) : null}
                <HealthBadge health={account.health} />
              </div>
            )}
            <p className="text-xs text-muted-foreground">
              <span className="font-mono">{account.host}</span>
              {' · '}
              {account.scopes
                ? t('gitConnectionsHub.scopes', { scopes: account.scopes })
                : t(`gitConnectionsHub.hosts.${account.provider}.access`)}
            </p>
            {account.detail && needsAction ? (
              <p className="text-xs text-muted-foreground">{account.detail}</p>
            ) : null}
          </div>
        </div>
        <dl className="flex gap-4">
          <Stat
            label={t('gitConnectionsHub.stats.repos')}
            value={account.repos}
          />
          <Stat
            label={t('gitConnectionsHub.stats.apps')}
            value={account.apps}
          />
        </dl>
      </div>
      <div className="flex flex-wrap items-center gap-2 pl-11">
        {needsAction ? (
          account.authorizePath ? (
            <Button
              type="button"
              size="sm"
              onClick={() =>
                window.location.assign(account.authorizePath ?? '')
              }
            >
              <PlugGlassIcon className="size-5" aria-hidden="true" />
              {t('gitConnections.reconnect')}
            </Button>
          ) : (
            <Button
              size="sm"
              render={<Link to={account.settingsPath} />}
              nativeButton={false}
            >
              <PlugGlassIcon className="size-5" aria-hidden="true" />
              {t('gitConnections.reconnect')}
            </Button>
          )
        ) : null}
        {account.connectionId ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setRenaming(true)}
          >
            <PencilGlassIcon className="size-5" aria-hidden="true" />
            {t('gitConnections.rename.action')}
          </Button>
        ) : null}
        <Button
          size="sm"
          variant="outline"
          render={<Link to={account.settingsPath} />}
          nativeButton={false}
        >
          <ExternalLinkGlassIcon className="size-5" aria-hidden="true" />
          {t('gitConnectionsHub.openSettings')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="destructive"
          onClick={() => setConfirmOpen(true)}
        >
          {t('gitConnections.disconnect.action')}
        </Button>
      </div>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        tone="destructive"
        title={t('gitConnections.disconnect.title', { label: account.label })}
        description={t('gitConnectionsHub.disconnect.description')}
        consequences={
          account.apps
            ? [t('gitConnections.disconnect.sources', { count: account.apps })]
            : undefined
        }
        confirmLabel={t('gitConnections.disconnect.action')}
        pendingLabel={t('gitConnections.disconnect.pending')}
        pending={disconnect.isPending}
        error={disconnect.error?.message ?? null}
        onConfirm={confirmDisconnect}
      />
    </li>
  )
}
