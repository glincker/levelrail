import { Link } from '@tanstack/react-router'
import {
  DatabaseIcon,
  HardDrivesIcon,
  StackIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'
import type {
  ProjectTopologyNode,
  ProjectTopologyStatus,
} from '../../types/projectTopology'

const KIND_ICON = {
  app: <StackIcon className="size-3.5" aria-hidden="true" />,
  database: <DatabaseIcon className="size-3.5" aria-hidden="true" />,
  volume: <HardDrivesIcon className="size-3.5" aria-hidden="true" />,
} as const

// Solid-fill dot, mirroring web/src/lib/appStatus.ts's STATUS_DOT_COLOR
// exactly: the same status vocabulary AppRow/DatabaseRow already use
// for their own list-row dot, reused here rather than invented again.
const STATUS_DOT_COLOR: Record<ProjectTopologyStatus['variant'], string> = {
  success: 'bg-green-500 dark:bg-green-400',
  destructive: 'bg-destructive',
  muted: 'bg-muted-foreground/40',
}

function CardBody({ node }: { node: ProjectTopologyNode }) {
  return (
    <>
      <span className="flex items-center gap-1.5 font-medium text-foreground">
        {node.status ? (
          <span
            className={cn(
              'size-2 shrink-0 rounded-full',
              STATUS_DOT_COLOR[node.status.variant],
            )}
            role="img"
            aria-label={node.status.label}
            title={node.status.label}
          />
        ) : null}
        <span className="text-muted-foreground">{KIND_ICON[node.kind]}</span>
        <span className="truncate">{node.label}</span>
      </span>
      {node.status ? (
        <Badge variant={node.status.variant} className="w-fit">
          {node.status.label}
        </Badge>
      ) : null}
    </>
  )
}

// A plain callback ref, not React's own Ref<T> (RefCallback | RefObject):
// RefObject is invariant in T, so a Ref<HTMLAnchorElement | HTMLDivElement>
// cannot narrow to either Link's Ref<HTMLAnchorElement> or a div's
// Ref<HTMLDivElement>. A bare callback type has no such restriction: a
// function accepting the wider union is assignable wherever a function
// accepting one branch of it is expected.
type AnchorRefCallback = (el: HTMLAnchorElement | HTMLDivElement | null) => void

// One app/database/volume box, anchor-registered via `anchorRef` so
// ProjectTopologyView can draw connector lines to/from it without either
// side needing to know the other's screen position ahead of render, the
// same pattern components/network/TopologyResourceCard.tsx already
// establishes for the whole-mesh topology page. A volume has no detail
// route of its own, so it renders as a plain anchor-less box instead of
// a dead link.
export function ProjectTopologyResourceCard({
  node,
  anchorRef,
}: {
  node: ProjectTopologyNode
  anchorRef: AnchorRefCallback
}) {
  const className = cn(
    'flex min-w-[11rem] flex-col gap-1 rounded-lg border border-border bg-card px-3 py-2 text-sm transition-colors',
    node.kind !== 'volume' && 'hover:border-ring',
  )

  if (node.kind === 'database') {
    return (
      <Link
        ref={anchorRef}
        to="/databases/$name"
        params={{ name: node.label }}
        className={className}
      >
        <CardBody node={node} />
      </Link>
    )
  }
  if (node.kind === 'app') {
    return (
      <Link
        ref={anchorRef}
        to="/apps/$name/overview"
        params={{ name: node.label }}
        className={className}
      >
        <CardBody node={node} />
      </Link>
    )
  }
  return (
    <div ref={anchorRef} className={className}>
      <CardBody node={node} />
    </div>
  )
}
