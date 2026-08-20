import {
  ArrowRightIcon,
  CloudIcon,
  GlobeIcon,
  PackageIcon,
  ShareNetworkIcon,
  WifiHighIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import type { AppDetail } from '../types/appDetail'
import { useAppNetwork } from '../queries/appNetwork'
import { DomainDnsCheck } from './DomainDnsCheck'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

// Renders the actual, live traffic path for an app: domain (if any)
// through the embedded Caddy ingress, to whatever host port Docker
// currently has this app's container bound to, down to the container's
// own declared port. GET /api/v1/apps/{name}/network
// (internal/api/network.go) supplies the host-port half; the domain
// half reuses DomainDnsCheck (queries/domainCheck.ts) exactly as
// DomainEditor already does, rather than a second implementation of the
// same DNS-routing check.
export function AppNetworkPanel({ app }: { app: AppDetail }) {
  const { data: network, isLoading } = useAppNetwork(app.name)
  const domains = app.domains ?? []
  const running = network?.running ?? false
  const hostPort = network?.host_port

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ShareNetworkIcon className="size-4" />
          Network
        </CardTitle>
        <CardDescription>
          How traffic actually reaches this app right now, from domain to
          container.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        <TrafficPath
          domains={domains}
          containerPort={app.port}
          hostPort={hostPort}
          running={running}
          isLoading={isLoading}
        />

        <div className="flex flex-wrap items-center gap-x-8 gap-y-3 text-sm">
          <FlowField label="Container port">
            <span className="font-mono">:{app.port}</span>
          </FlowField>
          <FlowField label="Host port">
            {running && hostPort ? (
              <span className="font-mono">:{hostPort}</span>
            ) : (
              <span className="text-muted-foreground italic">not running</span>
            )}
          </FlowField>
          <FlowField label="Status">
            <Badge variant={running ? 'success' : 'muted'}>
              {running ? 'Running' : 'Not running'}
            </Badge>
          </FlowField>
        </div>

        <div className="border-t border-border pt-4">
          <h3 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
            Domains
          </h3>
          {domains.length === 0 ? (
            <p className="mt-2 text-sm text-muted-foreground">
              No domain configured. This app is only reachable on this
              server&apos;s host port directly.
            </p>
          ) : (
            <div className="mt-3 space-y-2">
              {domains.map((domain) => (
                <DomainDnsCheck key={domain} appName={app.name} domain={domain} />
              ))}
            </div>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

function FlowField({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <div className="min-w-0">
      <dt className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {label}
      </dt>
      <dd className="mt-0.5 flex min-w-0 items-center text-sm text-foreground">
        {children}
      </dd>
    </div>
  )
}

// The at-a-glance chain: domain -> Caddy -> host:port -> container port,
// each hop separated by an arrow so an operator can read left to right
// exactly how a request travels, without needing to already know what
// Caddy or a Docker host-port mapping are.
function TrafficPath({
  domains,
  containerPort,
  hostPort,
  running,
  isLoading,
}: {
  domains: string[]
  containerPort: number
  hostPort?: number
  running: boolean
  isLoading: boolean
}) {
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-md border border-border bg-muted/30 p-3">
      <PathNode
        icon={GlobeIcon}
        label={domains[0] ?? 'No domain'}
        sublabel={domains.length > 1 ? `+${domains.length - 1} more` : undefined}
        muted={domains.length === 0}
      />
      <ArrowRightIcon className="size-4 shrink-0 text-muted-foreground" />
      <PathNode icon={CloudIcon} label="Caddy" sublabel="ingress" />
      <ArrowRightIcon className="size-4 shrink-0 text-muted-foreground" />
      <PathNode
        icon={WifiHighIcon}
        label={running && hostPort ? `:${hostPort}` : 'no host port'}
        sublabel="this machine"
        muted={!running || !hostPort}
      />
      <ArrowRightIcon className="size-4 shrink-0 text-muted-foreground" />
      <PathNode
        icon={PackageIcon}
        label={`:${containerPort}`}
        sublabel="container"
      />
      {isLoading ? (
        <span className="ml-auto text-xs text-muted-foreground">
          Checking live status...
        </span>
      ) : null}
    </div>
  )
}

function PathNode({
  icon: NodeIcon,
  label,
  sublabel,
  muted,
}: {
  icon: Icon
  label: string
  sublabel?: string
  muted?: boolean
}) {
  return (
    <div
      className={`flex items-center gap-2 rounded-md border px-2.5 py-1.5 ${
        muted
          ? 'border-dashed border-border bg-background text-muted-foreground'
          : 'border-border bg-background text-foreground'
      }`}
    >
      <NodeIcon className="size-4 shrink-0" />
      <div className="min-w-0 leading-tight">
        <p className="truncate text-xs font-medium">{label}</p>
        {sublabel ? (
          <p className="truncate text-[10px] text-muted-foreground">
            {sublabel}
          </p>
        ) : null}
      </div>
    </div>
  )
}
