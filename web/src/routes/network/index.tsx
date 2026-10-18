import { createFileRoute, Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  networkTopologyQueryOptions,
  useNetworkTopology,
} from '../../queries/networkTopology'
import { useMeshSettings } from '../../queries/meshSettings'
import { MeshConnectivityPanel } from '../../components/network/MeshConnectivityPanel'
import { MeshPeersPanel } from '../../components/network/MeshPeersPanel'
import { MeshSettingsCard } from '../../components/network/MeshSettingsCard'
import {
  NetworkSummaryRow,
  NetworkTopologyView,
} from '../../components/network/NetworkTopologyView'
import { routeErrorMessage } from '../../lib/apiError'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { buttonVariants } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader } from '@/components/shell/PageHeader'

// The whole-server network page: this server and any enrolled nodes as
// cards, the mesh switch, and the mesh health panels once the mesh runs.
// Grouping is done by the API (internal/api/network_topology_groups.go).
export const Route = createFileRoute('/network/')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(networkTopologyQueryOptions()),
  component: NetworkPage,
  pendingComponent: NetworkPageSkeleton,
  errorComponent: NetworkPageError,
})

function NetworkPage() {
  const { t } = useTranslation('network')
  const { data: topology } = useNetworkTopology()
  const { data: settings } = useMeshSettings()
  const localNodeId = topology.nodes.find((n) => n.is_local)?.id

  return (
    <div className="space-y-4">
      <PageHeader
        title={t('page.title')}
        description={t('page.description', { zone: topology.zone })}
        actions={
          <Link
            to="/network/private"
            className={buttonVariants({ variant: 'outline' })}
          >
            {t('privateLink')}
          </Link>
        }
      />

      <NetworkSummaryRow
        topology={topology}
        restartPending={Boolean(settings?.restart_required)}
      />

      <MeshSettingsCard />

      <NetworkTopologyView topology={topology} />

      {topology.mesh_enabled ? (
        <>
          <MeshPeersPanel localNodeId={localNodeId} />
          <MeshConnectivityPanel />
        </>
      ) : null}
    </div>
  )
}

function NetworkPageSkeleton() {
  return (
    <div className="space-y-4" aria-hidden="true">
      <div>
        <Skeleton className="h-6 w-24" />
        <Skeleton className="mt-1 h-4 w-72 max-w-full" />
      </div>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
        {Array.from({ length: 4 }, (_, i) => (
          <Skeleton key={i} className="h-14 rounded-lg" />
        ))}
      </div>
      <Skeleton className="h-40 w-full rounded-xl" />
      <div className="grid gap-3 md:grid-cols-2">
        {Array.from({ length: 2 }, (_, i) => (
          <Skeleton key={i} className="h-40 rounded-xl" />
        ))}
      </div>
    </div>
  )
}

function NetworkPageError({ error }: { error: unknown }) {
  const { t } = useTranslation('network')
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p>{t('error.load')}</p>
        <p>{routeErrorMessage(error)}</p>
      </AlertDescription>
    </Alert>
  )
}
