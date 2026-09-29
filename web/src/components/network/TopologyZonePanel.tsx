import { Link } from '@tanstack/react-router'
import { HardDrivesIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import type { TopologyZone } from '../../lib/networkTopologyLayout'
import { hasMeshAddress } from '../../lib/networkTopologyLayout'
import { TopologyResourceCard } from './TopologyResourceCard'

const STATUS_BADGE_VARIANT: Record<
  string,
  'success' | 'destructive' | 'muted'
> = {
  online: 'success',
  offline: 'destructive',
  pending: 'muted',
}

// One zone: a node's own border (region label, or the node's name when
// no region is set) with its apps, databases and load balancers inside.
// The trailing "Unplaced" zone (zone.node undefined) renders the same
// card grid without a node header, since it has no real node to describe.
export function TopologyZonePanel({
  zone,
  registerAnchor,
}: {
  zone: TopologyZone
  registerAnchor: (id: string) => (el: HTMLAnchorElement | null) => void
}) {
  const empty = zone.apps.length === 0 && zone.databases.length === 0

  return (
    <div className="flex min-w-[18rem] flex-1 flex-col gap-3 rounded-xl border border-dashed border-border bg-muted/20 p-3">
      <div className="flex items-center justify-between gap-2">
        {zone.node ? (
          <Link
            to="/nodes/$id"
            params={{ id: zone.node.id }}
            className="flex items-center gap-1.5 text-sm font-semibold text-foreground hover:underline"
          >
            <HardDrivesIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            {zone.label}
          </Link>
        ) : (
          <span className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
            <HardDrivesIcon
              className="size-4 text-muted-foreground"
              aria-hidden="true"
            />
            {zone.label}
          </span>
        )}
        {zone.node ? (
          <div className="flex items-center gap-1.5">
            <Badge variant={STATUS_BADGE_VARIANT[zone.node.status] ?? 'muted'}>
              {zone.node.status}
            </Badge>
            {!zone.node.schedulable ? (
              <Badge variant="warning">Cordoned</Badge>
            ) : null}
          </div>
        ) : null}
      </div>
      {zone.node ? (
        <div className="font-mono text-xs text-muted-foreground">
          {zone.node.mesh_address || 'Not on the mesh yet'}
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">
          Placed on a node this control plane no longer knows about.
        </p>
      )}

      {empty ? (
        <p className="text-xs text-muted-foreground">Nothing placed here.</p>
      ) : (
        <div className="flex flex-col gap-2">
          {zone.apps.map((app) => (
            <TopologyResourceCard
              key={`app:${app.name}`}
              kind="app"
              name={app.name}
              dnsName={app.dns_name}
              meshAddress={app.mesh_address}
              reachable={hasMeshAddress(app)}
              loadBalancerAlgorithm={
                zone.loadBalancerByApp.get(app.name)?.algorithm
              }
              domains={app.domains}
              anchorRef={registerAnchor(`app:${app.name}`)}
            />
          ))}
          {zone.databases.map((database) => (
            <TopologyResourceCard
              key={`db:${database.name}`}
              kind="database"
              name={database.name}
              dnsName={database.dns_name}
              meshAddress={database.mesh_address}
              reachable={hasMeshAddress(database)}
              anchorRef={registerAnchor(`db:${database.name}`)}
            />
          ))}
        </div>
      )}
    </div>
  )
}
