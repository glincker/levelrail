import type { ReactNode } from 'react'
import {
  HardDrivesIcon,
  PlugsConnectedIcon,
} from '@phosphor-icons/react/dist/ssr'
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
import { toast } from '@/components/ui/toast'
import { DeleteNetworkShareDialog } from './DeleteNetworkShareDialog'
import { useTestNetworkShare } from '../queries/networkShares'
import type { NetworkShare } from '../types/networkShare'

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

function remoteAddress(share: NetworkShare): string {
  return share.protocol === 'nfs'
    ? `${share.host}:${share.remote_path}`
    : `//${share.host}${share.remote_path}`
}

function TestButton({ share }: { share: NetworkShare }) {
  const testShare = useTestNetworkShare()
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={testShare.isPending}
      onClick={() => {
        testShare.mutate(share.id, {
          onSuccess: () => {
            toast.add({
              title: `"${share.name}" is reachable.`,
              type: 'success',
            })
          },
          onError: (error) => {
            toast.add({
              title: `"${share.name}" is not reachable.`,
              description: error.message,
              type: 'error',
            })
          },
        })
      }}
    >
      <PlugsConnectedIcon className="size-3.5" aria-hidden="true" />
      {testShare.isPending ? 'Testing...' : 'Test connection'}
    </Button>
  )
}

// A configured NFS/CIFS export plus its reachability test and delete
// action. No column for which services mount it: GET
// /api/v1/network-shares carries none of that, the same boundary
// RegistryCredentialTable draws for its own resource.
export function NetworkShareTable({
  shares,
  action,
}: {
  shares: NetworkShare[]
  action?: ReactNode
}) {
  if (shares.length === 0) {
    return (
      <EmptyState
        className="py-12"
        icon={<HardDrivesIcon className="size-5" />}
        title="No network shares"
        description="Add an NFS export or a CIFS/SMB share to mount as a Docker volume source, using Docker's own local-driver network passthrough."
        action={action}
      />
    )
  }

  return (
    <div className="rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Protocol</TableHead>
            <TableHead>Remote</TableHead>
            <TableHead>Added</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {shares.map((share) => (
            <TableRow key={share.id}>
              <TableCell className="font-medium text-foreground">
                {share.name}
              </TableCell>
              <TableCell>
                <Badge variant="outline" className="uppercase">
                  {share.protocol}
                </Badge>
              </TableCell>
              <TableCell className="font-mono text-muted-foreground">
                {remoteAddress(share)}
              </TableCell>
              <TableCell className="text-muted-foreground">
                {formatDate(share.created_at)}
              </TableCell>
              <TableCell className="text-right">
                <div className="flex justify-end gap-2">
                  <TestButton share={share} />
                  <DeleteNetworkShareDialog share={share} />
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
