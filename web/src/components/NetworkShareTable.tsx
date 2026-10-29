import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { PlugGlassIcon } from '@thesvg/ui-react'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
      <PlugGlassIcon className="size-4" aria-hidden="true" />
      {testShare.isPending ? 'Testing...' : 'Test connection'}
    </Button>
  )
}

// A configured NFS/CIFS export plus its reachability test and delete
// action. No column for which services mount it: GET
// /api/v1/network-shares carries none of that, the same boundary
// RegistryCredentialTable draws for its own resource.
export function NetworkShareTable({ shares }: { shares: NetworkShare[] }) {
  const { t } = useTranslation('common')
  const columns = useMemo<ResponsiveColumn<NetworkShare>[]>(
    () => [
      {
        id: 'name',
        header: 'Name',
        primary: true,
        cell: (s) => (
          <span className="font-medium text-foreground">{s.name}</span>
        ),
      },
      {
        id: 'protocol',
        header: 'Protocol',
        priority: 2,
        cell: (s) => (
          <Badge variant="outline" className="uppercase">
            {s.protocol}
          </Badge>
        ),
      },
      {
        id: 'remote',
        header: 'Remote',
        cell: (s) => (
          <span className="font-mono break-all text-muted-foreground">
            {remoteAddress(s)}
          </span>
        ),
      },
      {
        id: 'added',
        header: 'Added',
        priority: 3,
        cell: (s) => (
          <span className="text-muted-foreground">
            {formatDate(s.created_at)}
          </span>
        ),
      },
    ],
    [],
  )

  return (
    <ResponsiveTable
      rows={shares}
      columns={columns}
      getRowId={(s) => s.id}
      ariaLabel={t('tableLabels.networkShares')}
      renderActions={(s) => (
        <span className="flex flex-wrap justify-end gap-2">
          <TestButton share={s} />
          <DeleteNetworkShareDialog share={s} />
        </span>
      )}
    />
  )
}
