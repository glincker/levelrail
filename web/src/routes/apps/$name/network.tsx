import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { appNetworkQueryOptions } from '../../../queries/appNetwork'
import { AppNetworkPanel } from '../../../components/AppNetworkPanel'
import { PageSpinner } from '@/components/ui/page-spinner'

// The live traffic-path tab: domain through Caddy ingress to the
// Docker-assigned host port to the container's own declared port. Reads
// app data from the query cache the parent layout route's loader already
// primed; primes the network query here so the first paint doesn't show
// a loading flash for the host-port half.
export const Route = createFileRoute('/apps/$name/network')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    queryClient.ensureQueryData(appNetworkQueryOptions(name)),
  component: NetworkSection,
  pendingComponent: PageSpinner,
})

function NetworkSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  return <AppNetworkPanel app={app} />
}
