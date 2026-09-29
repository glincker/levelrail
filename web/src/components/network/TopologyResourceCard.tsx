import type { ReactNode, Ref } from 'react'
import { Link } from '@tanstack/react-router'
import {
  ArrowsSplitIcon,
  DatabaseIcon,
  StackIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

export type TopologyResourceKind = 'app' | 'database'

const KIND_ICON: Record<TopologyResourceKind, ReactNode> = {
  app: <StackIcon className="size-3.5" aria-hidden="true" />,
  database: <DatabaseIcon className="size-3.5" aria-hidden="true" />,
}

// One app or database card inside a TopologyZone, anchor-registered via
// `anchorRef` so NetworkTopologyView can draw connection lines to/from it
// without either side needing to know the other's screen position ahead
// of render.
function CardBody({
  kind,
  name,
  dnsName,
  meshAddress,
  reachable,
  loadBalancerAlgorithm,
  domains,
}: {
  kind: TopologyResourceKind
  name: string
  dnsName?: string
  meshAddress?: string
  reachable: boolean
  loadBalancerAlgorithm?: string
  domains?: string[]
}) {
  return (
    <>
      <span className="flex items-center gap-1.5 font-medium text-foreground">
        <span className="text-muted-foreground">{KIND_ICON[kind]}</span>
        <span className="truncate">{name}</span>
        {loadBalancerAlgorithm ? (
          <Badge variant="outline" className="ml-auto">
            <ArrowsSplitIcon className="size-3" aria-hidden="true" />
            {loadBalancerAlgorithm.replace('_', ' ')}
          </Badge>
        ) : null}
      </span>
      {dnsName ? (
        <span className="truncate font-mono text-xs text-muted-foreground">
          {dnsName}
          {meshAddress ? ` → ${meshAddress}` : ''}
        </span>
      ) : null}
      {domains && domains.length > 0 ? (
        <span className="truncate text-xs text-muted-foreground">
          {domains.join(', ')}
        </span>
      ) : null}
      {!reachable ? (
        <Badge variant="warning" className="w-fit">
          <WarningCircleIcon className="size-3" aria-hidden="true" />
          No mesh address
        </Badge>
      ) : null}
    </>
  )
}

// One app or database card inside a TopologyZone, anchor-registered via
// `anchorRef` so NetworkTopologyView can draw connection lines to/from it
// without either side needing to know the other's screen position ahead
// of render. Two literal <Link> branches, not one with a computed `to`:
// TanStack Router's typed routing needs `to` to be one of its own literal
// route strings, so a caller-supplied `to: string` would not type-check
// against it.
export function TopologyResourceCard({
  kind,
  name,
  dnsName,
  meshAddress,
  reachable,
  loadBalancerAlgorithm,
  domains,
  anchorRef,
}: {
  kind: TopologyResourceKind
  name: string
  dnsName?: string
  meshAddress?: string
  reachable: boolean
  loadBalancerAlgorithm?: string
  domains?: string[]
  anchorRef: Ref<HTMLAnchorElement>
}) {
  const className = cn(
    'flex flex-col gap-1 rounded-lg border bg-card px-3 py-2 text-sm transition-colors hover:border-ring',
    reachable
      ? 'border-border'
      : 'border-amber-400/60 dark:border-amber-500/40',
  )
  const body = (
    <CardBody
      kind={kind}
      name={name}
      dnsName={dnsName}
      meshAddress={meshAddress}
      reachable={reachable}
      loadBalancerAlgorithm={loadBalancerAlgorithm}
      domains={domains}
    />
  )

  if (kind === 'database') {
    return (
      <Link
        ref={anchorRef}
        to="/databases/$name"
        params={{ name }}
        className={className}
      >
        {body}
      </Link>
    )
  }
  return (
    <Link
      ref={anchorRef}
      to="/apps/$name/overview"
      params={{ name }}
      className={className}
    >
      {body}
    </Link>
  )
}
