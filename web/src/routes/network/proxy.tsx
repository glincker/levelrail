import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  networkProxyQueryOptions,
  useNetworkProxy,
} from '../../queries/networkProxy'
import { ProxyReachabilityTable } from '../../components/network/ProxyReachabilityTable'
import { ProxySetupCard } from '../../components/ProxySetupCard'
import { routeErrorMessage } from '../../lib/apiError'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { PageHeader } from '@/components/shell/PageHeader'

// The Traffic page: a flat, domain-by-domain reachability table rather
// than a topology diagram (that's /network's own job, see
// NetworkTopologyView). Built for one specific, otherwise-invisible
// failure mode: an app placed on a node this control plane's own
// embedded ingress can never route to (CrossNodeIngress, see
// internal/reconcile/ingress/cross_node.go and
// internal/api/doctor_cross_node_ingress.go), surfaced here per domain
// with a direct fix instead of only as a flat doctor pass/fail line.
export const Route = createFileRoute('/network/proxy')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(networkProxyQueryOptions()),
  component: ProxyPage,
  pendingComponent: ProxyPagePending,
  errorComponent: ProxyPageError,
})

function ProxyPage() {
  const { t } = useTranslation('networkProxy')
  const { data } = useNetworkProxy()

  return (
    <div className="space-y-4">
      <PageHeader title={t('page.title')} description={t('page.description')} />
      <ProxySetupCard />
      <ProxyReachabilityTable domains={data.domains} />
    </div>
  )
}

function ProxyPagePending() {
  return (
    <div className="space-y-4" aria-hidden="true">
      <div>
        <Skeleton className="h-6 w-24" />
        <Skeleton className="mt-1 h-4 w-96" />
      </div>
      <div className="space-y-2 rounded-lg border border-border p-4">
        {Array.from({ length: 4 }, (_, i) => (
          <Skeleton key={i} className="h-10 w-full" />
        ))}
      </div>
    </div>
  )
}

function ProxyPageError({ error }: { error: unknown }) {
  return (
    <Alert variant="destructive">
      <AlertDescription>
        <p>{routeErrorMessage(error)}</p>
      </AlertDescription>
    </Alert>
  )
}
