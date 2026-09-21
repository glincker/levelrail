import { createFileRoute } from '@tanstack/react-router'
import { useApp } from '../../../queries/apps'
import { appNetworkQueryOptions } from '../../../queries/appNetwork'
import { AppNetworkPanel } from '../../../components/AppNetworkPanel'
import { AppEgressPolicyCard } from '../../../components/AppEgressPolicyCard'
import { PageSpinner } from '@/components/ui/page-spinner'

// The live traffic-path tab: domain through Caddy ingress to the
// Docker-assigned host port to the container's own declared port (inbound,
// AppNetworkPanel), plus this app's own outbound egress allowlist
// (AppEgressPolicyCard), the other half of "network" for this app. Reads
// app data from the query cache the parent layout route's loader already
// primed; primes the network query here so the first paint doesn't show
// a loading flash for the host-port half. EgressPolicyReady conditions are
// already primed by the parent layout route too (routes/apps/$name.tsx),
// so AppEgressPolicyCard needs no loader entry of its own.
export const Route = createFileRoute('/apps/$name/network')({
  loader: ({ context: { queryClient }, params: { name } }) =>
    queryClient.ensureQueryData(appNetworkQueryOptions(name)),
  component: NetworkSection,
  pendingComponent: PageSpinner,
})

function NetworkSection() {
  const { name } = Route.useParams()
  const { data: app } = useApp(name)

  return (
    <div className="space-y-6">
      <AppNetworkPanel app={app} />
      <AppEgressPolicyCard appName={name} />
    </div>
  )
}
