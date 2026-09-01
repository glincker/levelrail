import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { TerminalWindowIcon } from '@phosphor-icons/react/dist/ssr'
import { toast } from '@/components/ui/toast'
import {
  useApproveDeviceAuthRequest,
  useDenyDeviceAuthRequest,
} from '../queries/deviceAuth'
import type { DeviceAuthRequest } from '../queries/deviceAuth'

function formatRequestedAgo(iso: string): string {
  const diffSec = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000))
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
  const approve = useApproveDeviceAuthRequest()
  const deny = useDenyDeviceAuthRequest()

  if (requests.length === 0) {
    return (
      <EmptyState
        icon={<TerminalWindowIcon className="size-5" />}
        title="No pending CLI logins"
        description="Run levelrail-cli auth login --device in a terminal and the code will show up here."
      />
    )
  }

  return (
    <div className="rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Code</TableHead>
            <TableHead>Device</TableHead>
            <TableHead>Requested</TableHead>
            <TableHead>Expires</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {requests.map((request) => {
            const isHighlighted = highlightUserCode === request.user_code
            const isDeciding =
              (approve.isPending && approve.variables === request.user_code) ||
              (deny.isPending && deny.variables === request.user_code)
            return (
              <TableRow
                key={request.user_code}
                className={isHighlighted ? 'bg-primary/5' : undefined}
              >
                <TableCell className="font-mono font-medium text-foreground">
                  {request.user_code}
                </TableCell>
                <TableCell>
                  {request.client_name ? (
                    request.client_name
                  ) : (
                    <span className="text-muted-foreground">Unknown device</span>
                  )}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {formatRequestedAgo(request.created_at)}
                </TableCell>
                <TableCell>
                  <Badge variant="muted">{formatExpiresIn(request.expires_at)}</Badge>
                </TableCell>
                <TableCell className="text-right">
                  <div className="flex items-center justify-end gap-2">
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
                        approve.mutate(request.user_code, {
                          onSuccess: () => {
                            toast.add({
                              title: `Approved login for "${request.client_name || 'Unknown device'}".`,
                              type: 'success',
                            })
                          },
                          onError: (error) => {
                            toast.add({ title: error.message, type: 'error' })
                          },
                        })
                      }}
                    >
                      Approve
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
