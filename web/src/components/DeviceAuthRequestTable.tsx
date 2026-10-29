import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { TerminalGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { DeviceLoginConfirmDialog } from './attention/DeviceLoginConfirmDialog'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { toast } from '@/components/ui/toast'
import {
  useApproveDeviceAuthRequest,
  useDenyDeviceAuthRequest,
} from '../queries/deviceAuth'
import type { DeviceAuthRequest } from '../queries/deviceAuth'

function formatRequestedAgo(iso: string): string {
  const diffSec = Math.max(
    0,
    Math.round((Date.now() - new Date(iso).getTime()) / 1000),
  )
  if (diffSec < 5) return 'just now'
  if (diffSec < 60) return `${diffSec}s ago`
  const diffMin = Math.round(diffSec / 60)
  if (diffMin < 60) return `${diffMin}m ago`
  return `${Math.round(diffMin / 60)}h ago`
}

function formatExpiresIn(iso: string): string {
  const diffSec = Math.round((new Date(iso).getTime() - Date.now()) / 1000)
  if (diffSec <= 0) return 'expiring'
  if (diffSec < 60) return `${diffSec}s`
  return `${Math.round(diffSec / 60)}m`
}

// One row per pending code, split out of the route so the route stays a
// thin loader + layout shell, the same split TokenTable/UserTable use.
// A request drops off GET /api/v1/auth/device/requests the moment it's
// decided (device_auth.go's own ListPendingDeviceAuthRequests filters
// server-side), so approve/deny only need to invalidate the list query,
// never remove the row optimistically themselves.
export function DeviceAuthRequestTable({
  requests,
  highlightUserCode,
}: {
  requests: DeviceAuthRequest[]
  highlightUserCode?: string
}) {
  const { t } = useTranslation('attention')
  const approve = useApproveDeviceAuthRequest()
  const deny = useDenyDeviceAuthRequest()
  const [confirming, setConfirming] = useState<DeviceAuthRequest | null>(null)

  if (requests.length === 0) {
    return (
      <EmptyState
        compact
        icon={<TerminalGlassIcon className="size-5" />}
        title={t('deviceLogin.emptyTitle')}
        description={t('deviceLogin.emptyBody')}
      />
    )
  }

  const columns: ResponsiveColumn<DeviceAuthRequest>[] = [
    {
      id: 'code',
      primary: true,
      header: 'Code',
      cell: (request) => (
        <span className="font-mono font-medium text-foreground">
          {request.user_code}
        </span>
      ),
    },
    {
      id: 'device',
      header: 'Device',
      cell: (request) =>
        request.client_name ? (
          request.client_name
        ) : (
          <span className="text-muted-foreground">Unknown device</span>
        ),
    },
    {
      id: 'from',
      header: 'From',
      priority: 2,
      cell: (request) => (
        <span className="text-muted-foreground">
          <span className="font-mono text-xs">
            {request.requester_ip || '-'}
          </span>
          {request.ip_mismatch ? (
            <span
              className="ml-1.5 inline-flex items-center gap-1 text-xs font-medium text-destructive"
              title={t('deviceLogin.ipMismatch', { ip: request.requester_ip })}
            >
              <TriangleAlertGlassIcon
                aria-hidden="true"
                className="size-6 text-tone-warning"
              />
              {t('deviceLogin.review')}
            </span>
          ) : null}
        </span>
      ),
    },
    {
      id: 'requested',
      header: 'Requested',
      priority: 3,
      cell: (request) => (
        <span className="text-muted-foreground">
          {formatRequestedAgo(request.created_at)}
        </span>
      ),
    },
    {
      id: 'expires',
      header: 'Expires',
      priority: 2,
      cell: (request) => (
        <Badge variant="muted">{formatExpiresIn(request.expires_at)}</Badge>
      ),
    },
  ]

  return (
    <>
      <ResponsiveTable
        rows={requests}
        columns={columns}
        getRowId={(request) => request.user_code}
        ariaLabel={t('deviceLogin.tableLabel')}
        getRowProps={(request) => ({
          className:
            highlightUserCode === request.user_code
              ? 'bg-primary/5'
              : undefined,
        })}
        renderActions={(request) => {
          const isDeciding =
            (approve.isPending && approve.variables === request.user_code) ||
            (deny.isPending && deny.variables === request.user_code)
          return (
            <div className="flex flex-wrap items-center justify-end gap-2">
              <Button
                type="button"
                variant="destructive"
                size="sm"
                disabled={isDeciding}
                onClick={() => {
                  deny.mutate(request.user_code, {
                    onSuccess: () => {
                      toast.add({
                        title: `Denied login for "${request.client_name || 'Unknown device'}".`,
                        type: 'success',
                      })
                    },
                    onError: (error) => {
                      toast.add({ title: error.message, type: 'error' })
                    },
                  })
                }}
              >
                Deny
              </Button>
              <Button
                type="button"
                variant="default"
                size="sm"
                disabled={isDeciding}
                onClick={() => {
                  setConfirming(request)
                }}
              >
                {t('deviceLogin.review')}
              </Button>
            </div>
          )
        }}
      />
      {confirming ? (
        <DeviceLoginConfirmDialog
          request={confirming}
          open
          onOpenChange={(next) => {
            if (!next) {
              setConfirming(null)
            }
          }}
        />
      ) : null}
    </>
  )
}
