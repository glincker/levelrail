import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleXGlassIcon,
  KeyGlassIcon,
  PencilGlassIcon,
  PlugGlassIcon,
  StarGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { StatusBadge } from '@/components/ui/status-badge'
import { toast } from '@/components/ui/toast'
import {
  useDisconnectGitConnection,
  useRenameGitConnection,
  useSetDefaultGitConnection,
} from '../../queries/gitConnections'
import type { GitProviderConnection } from '../../types/gitConnections'

function ConnectionStateBadge({
  connection,
}: Readonly<{ connection: GitProviderConnection }>) {
  const { t } = useTranslation('settings')
  if (!connection.authorized) {
    return (
      <StatusBadge
        tone="warning"
        icon={KeyGlassIcon}
        label={t('gitConnections.state.notAuthorized')}
      />
    )
  }
  switch (connection.connection.status) {
    case 'needs_reauthorize':
    case 'revoked':
      return (
        <StatusBadge
          tone="danger"
          icon={CircleXGlassIcon}
          label={t('gitConnections.state.needsReconnect')}
        />
      )
    case 'rate_limited':
      return (
        <StatusBadge
          tone="warning"
          icon={TriangleAlertGlassIcon}
          label={t('gitConnections.state.rateLimited')}
        />
      )
    default:
      return (
        <StatusBadge
          tone="success"
          icon={CircleCheckGlassIcon}
          label={t('gitConnections.state.ok')}
        />
      )
  }
}

// One connected account: who it is, where, how healthy, and what you can do.
export function GitConnectionRow({
  connection,
  canSetDefault,
}: Readonly<{ connection: GitProviderConnection; canSetDefault: boolean }>) {
  const { t } = useTranslation('settings')
  const rename = useRenameGitConnection()
  const setDefault = useSetDefaultGitConnection()
  const disconnect = useDisconnectGitConnection()
  const [renaming, setRenaming] = useState(false)
  const [label, setLabel] = useState(connection.label)
  const [confirmOpen, setConfirmOpen] = useState(false)

  const needsAuthorization =
    !connection.authorized ||
    connection.connection.status === 'needs_reauthorize' ||
    connection.connection.status === 'revoked'

  function submitRename() {
    const next = label.trim()
    if (next === '' || next === connection.label) {
      setRenaming(false)
      return
    }
    rename.mutate(
      { id: connection.id, label: next },
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

  return (
    <li className="space-y-3 py-4 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-start justify-between gap-3">
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
                className="h-8 w-64"
              />
              <Button type="submit" size="sm" disabled={rename.isPending}>
                {t('gitConnections.rename.save')}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => {
                  setLabel(connection.label)
                  setRenaming(false)
                }}
              >
                {t('gitConnections.rename.cancel')}
              </Button>
            </form>
          ) : (
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm font-medium text-foreground">
                {connection.label}
              </span>
              {connection.is_default ? (
                <Badge variant="muted">{t('gitConnections.default')}</Badge>
              ) : null}
              <ConnectionStateBadge connection={connection} />
            </div>
          )}
          <p className="text-xs text-muted-foreground">
            {connection.account_name
              ? t('gitConnections.account', { name: connection.account_name })
              : t('gitConnections.accountUnknown')}
            {' · '}
            <span className="font-mono">{connection.host}</span>
            {' · '}
            {t('gitConnections.sources', { count: connection.source_count })}
          </p>
          {connection.connection.detail ? (
            <p className="text-xs text-muted-foreground">
              {connection.connection.detail}
            </p>
          ) : null}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {connection.authorize_path ? (
            <Button
              type="button"
              size="sm"
              variant={needsAuthorization ? 'default' : 'outline'}
              onClick={() => window.location.assign(connection.authorize_path)}
            >
              <PlugGlassIcon className="size-5" aria-hidden="true" />
              {connection.authorized
                ? t('gitConnections.reconnect')
                : t('gitConnections.authorize')}
            </Button>
          ) : null}
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => setRenaming(true)}
          >
            <PencilGlassIcon className="size-5" aria-hidden="true" />
            {t('gitConnections.rename.action')}
          </Button>
          {canSetDefault && !connection.is_default && connection.authorized ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={setDefault.isPending}
              onClick={() => setDefault.mutate(connection.id)}
            >
              <StarGlassIcon className="size-5" aria-hidden="true" />
              {t('gitConnections.setDefault')}
            </Button>
          ) : null}
          <Button
            type="button"
            size="sm"
            variant="destructive"
            onClick={() => setConfirmOpen(true)}
          >
            {t('gitConnections.disconnect.action')}
          </Button>
        </div>
      </div>
      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        tone="destructive"
        title={t('gitConnections.disconnect.title', {
          label: connection.label,
        })}
        description={t('gitConnections.disconnect.description')}
        consequences={
          connection.source_count > 0
            ? [
                t('gitConnections.disconnect.sources', {
                  count: connection.source_count,
                }),
              ]
            : undefined
        }
        confirmLabel={t('gitConnections.disconnect.action')}
        pendingLabel={t('gitConnections.disconnect.pending')}
        pending={disconnect.isPending}
        error={disconnect.error?.message ?? null}
        onConfirm={() =>
          disconnect.mutate(connection.id, {
            onSuccess: (result) => {
              setConfirmOpen(false)
              toast.add({
                title: t('gitConnections.toast.disconnected'),
                description: result.note,
                type: 'success',
              })
            },
          })
        }
      />
    </li>
  )
}
