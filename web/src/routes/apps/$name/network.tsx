import { createFileRoute } from '@tanstack/react-router'
import { ShareNetworkIcon } from '@phosphor-icons/react/dist/ssr'
import { useApp } from '../../../queries/apps'
import { appNetworkQueryOptions } from '../../../queries/appNetwork'
import { AppNetworkPanel } from '../../../components/AppNetworkPanel'
import { AppEgressPolicyCard } from '../../../components/AppEgressPolicyCard'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'

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
  pendingComponent: NetworkSectionSkeleton,
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

// Mirrors AppNetworkPanel's traffic-path card and AppEgressPolicyCard
// below it, the two cards this route actually renders.
function NetworkSectionSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ShareNetworkIcon className="size-4" />
            Network
          </CardTitle>
          <Skeleton className="h-4 w-72" />
        </CardHeader>
        <CardContent className="space-y-5">
          <Skeleton className="h-16 w-full rounded-lg" />
          <div className="flex flex-wrap gap-x-8 gap-y-3">
            {Array.from({ length: 3 }, (_, i) => (
              <div key={i} className="space-y-1">
                <Skeleton className="h-3 w-20" />
                <Skeleton className="h-4 w-16" />
              </div>
            ))}
          </div>
          <div className="space-y-2 border-t border-border pt-4">
            <Skeleton className="h-3 w-16" />
            <Skeleton className="h-4 w-56" />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>
            <Skeleton className="h-4 w-32" />
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <Skeleton className="h-4 w-full" />
          <Skeleton className="h-4 w-2/3" />
        </CardContent>
      </Card>
    </div>
  )
}
